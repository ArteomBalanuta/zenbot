package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/model"
	"zenbot/internal/repository"
)

var ErrMailDeliveryUnknown = errors.New("mail delivery outcome unknown; automatic replay prohibited")

// ErrMailSendNotStarted may be returned by a sender only when transport was
// definitely never entered. A transport error, even cancellation, is ambiguous.
var ErrMailSendNotStarted = errors.New("mail transport not started")

// MailDeliveryUncertainError identifies a durable attempt without exposing its
// private recipient or payload. The underlying transport/persistence errors remain
// available to errors.Is/As. ATTEMPTING and UNKNOWN must never be reclaimed.
type MailDeliveryUncertainError struct {
	MailID    int64
	AttemptID string
	Cause     error
}

func (e *MailDeliveryUncertainError) Error() string {
	return fmt.Sprintf("%s (mail %d, attempt %s): %v", ErrMailDeliveryUnknown, e.MailID, e.AttemptID, e.Cause)
}
func (e *MailDeliveryUncertainError) Unwrap() error        { return e.Cause }
func (e *MailDeliveryUncertainError) Is(target error) bool { return target == ErrMailDeliveryUnknown }

// DeliverPending owns one durable recipient attempt at a time. Sends are awaited
// synchronously; only a confirmed claim may reach transport. There are deliberately
// no retries or claim expiry: a crash after claim leaves an investigable attempt.
func (s *MailService) DeliverPending(ctx context.Context, receiver, trip string, send func(context.Context, model.Mail) error) error {
	mails, err := s.Pending(ctx, receiver, trip)
	if err != nil {
		return err
	}
	for _, mail := range mails {
		if err := ctx.Err(); err != nil {
			return err
		}
		attempt, claimed, err := s.claimMail(ctx, mail.ID, trip)
		if err != nil {
			if errors.Is(err, repository.ErrCommitOutcomeUnknown) {
				return &MailDeliveryUncertainError{mail.ID, attempt, err}
			}
			return err
		}
		if !claimed {
			continue
		}
		// Cancellation after a confirmed claim but before transport is definite.
		// Preserve a pending row, matching only this never-sent attempt identity.
		if err := ctx.Err(); err != nil {
			cleanup, cancel := mailCleanupContext(ctx)
			releaseErr := s.setMailAttempt(cleanup, mail.ID, trip, attempt, "PENDING")
			cancel()
			if releaseErr != nil {
				return &MailDeliveryUncertainError{mail.ID, attempt, errors.Join(err, releaseErr)}
			}
			return err
		}
		sendErr := send(ctx, mail)
		// A canceled caller must not discard the receipt for a completed send.
		// This bounded, synchronous cleanup never starts another transport send.
		cleanup, cancel := mailCleanupContext(ctx)
		state := "ACCEPTED"
		if sendErr != nil {
			state = "UNKNOWN"
		}
		if errors.Is(sendErr, ErrMailSendNotStarted) {
			state = "PENDING"
		}
		persistErr := s.setMailAttempt(cleanup, mail.ID, trip, attempt, state)
		cancel()
		if state == "PENDING" && persistErr == nil {
			return sendErr
		}
		if sendErr != nil || persistErr != nil {
			// A finalization rollback leaves ATTEMPTING. Best-effort recording of
			// UNKNOWN cannot overwrite an already committed ACCEPTED receipt.
			if persistErr != nil {
				cleanup, cancel := mailCleanupContext(ctx)
				unknownErr := s.markMailUnknown(cleanup, mail.ID, trip, attempt)
				cancel()
				persistErr = errors.Join(persistErr, unknownErr)
			}
			return &MailDeliveryUncertainError{mail.ID, attempt, errors.Join(sendErr, persistErr)}
		}
	}
	return ctx.Err()
}

func mailCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}

func (s *MailService) claimMail(ctx context.Context, id int64, trip string) (string, bool, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", false, err
	}
	attempt := hex.EncodeToString(token[:])
	claimed := false
	err := repository.WithTx(ctx, s.DB, func(tx *sql.Tx) error {
		// A source row lock serializes claims and aggregate finalization even
		// across independent service instances and different recipients.
		var recipients, status string
		if err := tx.QueryRowContext(ctx, `SELECT receiver,status FROM mail WHERE id=$1 FOR UPDATE`, id).Scan(&recipients, &status); err != nil {
			return err
		}
		if status != "PENDING" || !mailHasRecipient(recipients, trip) {
			return nil
		}
		var state string
		err := tx.QueryRowContext(ctx, `SELECT state FROM mail_delivery WHERE mail_id=$1 AND recipient_trip=$2`, id, trip).Scan(&state)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			_, err = tx.ExecContext(ctx, `INSERT INTO mail_delivery(mail_id,recipient_trip,attempt_id,state,updated_on) VALUES($1,$2,$3,'ATTEMPTING',$4)`, id, trip, attempt, time.Now().UnixMilli())
		case err != nil:
			return err
		case state == "PENDING":
			_, err = tx.ExecContext(ctx, `UPDATE mail_delivery SET attempt_id=$3,state='ATTEMPTING',updated_on=$4 WHERE mail_id=$1 AND recipient_trip=$2 AND state='PENDING'`, id, trip, attempt, time.Now().UnixMilli())
		default:
			return nil
		}
		claimed = err == nil
		return err
	})
	return attempt, claimed && err == nil, err
}

func mailHasRecipient(recipients, trip string) bool {
	if strings.TrimSpace(trip) == "" || strings.Contains(trip, ",") {
		return false
	}
	for _, recipient := range strings.Split(recipients, ",") {
		if recipient == trip {
			return true
		}
	}
	return false
}

func (s *MailService) setMailAttempt(ctx context.Context, id int64, trip, attempt, state string) error {
	return repository.WithTx(ctx, s.DB, func(tx *sql.Tx) error {
		var recipients string
		if err := tx.QueryRowContext(ctx, `SELECT receiver FROM mail WHERE id=$1 FOR UPDATE`, id).Scan(&recipients); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE mail_delivery SET state=$4,updated_on=$5 WHERE mail_id=$1 AND recipient_trip=$2 AND attempt_id=$3 AND state='ATTEMPTING'`, id, trip, attempt, state, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("mail attempt ownership lost for mail %d", id)
		}
		if state != "ACCEPTED" {
			return nil
		}
		for _, recipient := range strings.Split(recipients, ",") {
			var accepted int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM mail_delivery WHERE mail_id=$1 AND recipient_trip=$2 AND state='ACCEPTED'`, id, recipient).Scan(&accepted); err != nil {
				return err
			}
			if accepted != 1 {
				return nil
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE mail SET status='DELIVERED' WHERE id=$1 AND status='PENDING'`, id)
		return err
	})
}

func (s *MailService) markMailUnknown(ctx context.Context, id int64, trip, attempt string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE mail_delivery SET state='UNKNOWN',updated_on=$4 WHERE mail_id=$1 AND recipient_trip=$2 AND attempt_id=$3 AND state='ATTEMPTING'`, id, trip, attempt, time.Now().UnixMilli())
	return err
}
