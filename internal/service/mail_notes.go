package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

var (
	ErrMailReceiverBlank         = errors.New("receiver cannot be blank")
	ErrMailRecipientUnregistered = errors.New("user not registered")
)

type MailService struct {
	DB     *sql.DB
	Out    CommandOutput
	GroupB repository.SqlUtilGroupBRepository
}

func (s *MailService) Queue(ctx context.Context, message, owner, receiver string, whisper bool) error {
	_, err := s.QueueResolved(ctx, message, owner, receiver, whisper)
	return err
}

// QueueResolved persists pending mail and returns the resolved recipient trips
// used by Saturn's scheduling acknowledgement.
func (s *MailService) QueueResolved(ctx context.Context, message, owner, receiver string, whisper bool) (string, error) {
	receiver = strings.TrimPrefix(strings.TrimSpace(receiver), "@")
	if receiver == "" {
		return "", ErrMailReceiverBlank
	}
	// An exact registered trip takes precedence over a nickname with the same
	// spelling. Trip identities are case-sensitive; only nickname lookup folds case.
	rows, e := s.DB.QueryContext(ctx, `SELECT DISTINCT t.trip FROM trips t
		WHERE t.trip=$2 OR (NOT EXISTS (SELECT 1 FROM trips WHERE trip=$2) AND EXISTS (
			SELECT 1 FROM trip_names tn INNER JOIN names n ON tn.name_id=n.id
			WHERE tn.trip_id=t.id AND LOWER(n.name)=$1
		)) ORDER BY t.trip`, strings.ToLower(receiver), receiver)
	if e != nil {
		return "", e
	}
	var trips []string
	for rows.Next() {
		var trip string
		if e = rows.Scan(&trip); e != nil {
			rows.Close()
			return "", e
		}
		trips = append(trips, trip)
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return "", e
	}
	rows.Close()
	if len(trips) == 0 {
		return "", ErrMailRecipientUnregistered
	}
	receivers := strings.Join(trips, ",")
	if message != "" {
		message += " "
	}
	escapedMessage, _ := json.Marshal(message)
	message = string(escapedMessage[1 : len(escapedMessage)-1])
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES($1,$2,$3,'PENDING',$4,$5)`, owner, receivers, message, time.Now().UnixMilli(), strconv.FormatBool(whisper)); err != nil {
		return "", err
	}
	common.RecordCommittedMutation(ctx)
	return receivers, nil
}
func (s *MailService) RegisteredUsers(ctx context.Context) string {
	rows, e := s.DB.QueryContext(ctx, `SELECT DISTINCT n.name,t.trip FROM trip_names tn INNER JOIN trips t ON tn.trip_id=t.id INNER JOIN names n ON tn.name_id=n.id ORDER BY t.trip DESC`)
	if e != nil {
		return ""
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var name, trip string
		if rows.Scan(&name, &trip) == nil {
			b.WriteString(name)
			b.WriteByte(' ')
			b.WriteString(trip)
			b.WriteString("\\n")
		}
	}
	return b.String()
}

// SaturnRegisteredUsers exposes the Saturn-shaped compatibility read without
// changing the existing formatted directory contract.
func (s *MailService) SaturnRegisteredUsers(ctx context.Context) ([]repository.SaturnRegisteredUser, error) {
	if s.GroupB == nil {
		return nil, fmt.Errorf("group B repository unavailable")
	}
	return s.GroupB.SaturnRegisteredUsers(ctx)
}

func (s *MailService) Pending(ctx context.Context, receiver, trip string) ([]model.Mail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Receivers are resolved trips, never claimant-controlled nicknames. Keep
	// the nickname argument for callers, but authenticate delivery only by trip.
	if strings.TrimSpace(trip) == "" || strings.Contains(trip, ",") {
		return nil, nil
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT id,owner,receiver,message,status,created_on,is_whisper FROM mail WHERE status='PENDING' AND NOT EXISTS (SELECT 1 FROM mail_delivery d WHERE d.mail_id=mail.id AND d.recipient_trip=$1 AND d.state<>'PENDING') AND LOCATE(',' || $1 || ',', ',' || receiver || ',') > 0 ORDER BY id`, trip)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Mail
	for rows.Next() {
		var m model.Mail
		var w string
		if e = rows.Scan(&m.ID, &m.Owner, &m.Receiver, &m.Message, &m.Status, &m.CreatedOn, &w); e != nil {
			return nil, e
		}
		m.IsWhisper = strings.EqualFold(w, "true")
		out = append(out, m)
	}
	return out, rows.Err()
}

type NoteService struct {
	DB  *sql.DB
	Out CommandOutput
}

func (s *NoteService) Save(ctx context.Context, trip, note string) error {
	_, e := s.DB.ExecContext(ctx, `INSERT INTO notes(trip,note,created_on) VALUES($1,$2,$3)`, trip, note, time.Now().UnixMilli())
	if e == nil {
		common.RecordCommittedMutation(ctx)
	}
	return e
}
func (s *NoteService) List(ctx context.Context, trip string) ([]string, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT note FROM notes WHERE trip=$1 ORDER BY id`, trip)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var o []string
	for rows.Next() {
		var n string
		if e = rows.Scan(&n); e != nil {
			return nil, e
		}
		b, _ := json.Marshal(n)
		o = append(o, string(b[1:len(b)-1]))
	}
	return o, rows.Err()
}
func (s *NoteService) Clear(ctx context.Context, trip string) error {
	result, e := s.DB.ExecContext(ctx, `DELETE FROM notes WHERE trip=$1`, trip)
	if e == nil {
		if rows, err := result.RowsAffected(); err == nil && rows > 0 {
			common.RecordCommittedMutation(ctx)
		}
	}
	return e
}
