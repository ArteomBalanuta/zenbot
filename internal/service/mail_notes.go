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
	"zenbot/internal/util"
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
	receiver = strings.TrimSpace(receiver)
	nick, err := util.NormalizeNickTarget(&receiver)
	if err != nil {
		return "", ErrMailReceiverBlank
	}
	// An exact registered trip takes precedence over a nickname with the same
	// spelling. Trip identities are case-sensitive; only nickname lookup folds case.
	rows, e := s.DB.QueryContext(ctx, `SELECT DISTINCT t.trip FROM trips t
		WHERE t.trip=?2 OR (NOT EXISTS (SELECT 1 FROM trips WHERE trip=?2) AND EXISTS (
			SELECT 1 FROM trip_names tn INNER JOIN names n ON tn.name_id=n.id
			WHERE tn.trip_id=t.id AND LOWER(n.name)=?1
		)) ORDER BY t.trip`, strings.ToLower(nick), receiver)
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
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper,text_encoding) VALUES(?1,?2,?3,'PENDING',?4,?5,'PLAIN')`, owner, receivers, message, time.Now().UnixMilli(), strconv.FormatBool(whisper)); err != nil {
		return "", err
	}
	common.RecordCommittedMutation(ctx)
	return receivers, nil
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
	rows, e := s.DB.QueryContext(ctx, `SELECT id,owner,receiver,message,status,created_on,is_whisper,text_encoding FROM mail WHERE status='PENDING' AND NOT EXISTS (SELECT 1 FROM mail_delivery d WHERE d.mail_id=mail.id AND d.recipient_trip=?1 AND d.state<>'PENDING') AND instr(',' || receiver || ',', ',' || ?1 || ',') > 0 ORDER BY id`, trip)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Mail
	for rows.Next() {
		var m model.Mail
		var w, encoding string
		if e = rows.Scan(&m.ID, &m.Owner, &m.Receiver, &m.Message, &m.Status, &m.CreatedOn, &w, &encoding); e != nil {
			return nil, e
		}
		// Old Go and Saturn writers persist JSON string contents without quotes.
		// Decode only rows tagged with that contract; never sniff backslashes.
		switch encoding {
		case "JSON_STRING":
			if err := json.Unmarshal([]byte(`"`+m.Message+`"`), &m.Message); err != nil {
				return nil, fmt.Errorf("mail %d has invalid legacy text encoding", m.ID)
			}
		case "PLAIN":
		default:
			return nil, fmt.Errorf("mail %d has unsupported text encoding", m.ID)
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
	_, e := s.DB.ExecContext(ctx, `INSERT INTO notes(trip,note,created_on) VALUES(?1,?2,?3)`, trip, note, time.Now().UnixMilli())
	if e == nil {
		common.RecordCommittedMutation(ctx)
	}
	return e
}
func (s *NoteService) List(ctx context.Context, trip string) ([]string, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT note FROM notes WHERE trip=?1 ORDER BY id`, trip)
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
		o = append(o, n)
	}
	return o, rows.Err()
}
func (s *NoteService) Clear(ctx context.Context, trip string) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM notes WHERE trip=?1`, trip)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows > 0 {
		common.RecordCommittedMutation(ctx)
	}
	return nil
}
