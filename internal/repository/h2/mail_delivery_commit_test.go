package h2

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

// The actual SQL and transactions run on H2. Only the driver's response after
// Commit is controlled, to model a lost acknowledgement or caller cancellation.
type mailCommitConnector struct {
	driver.Connector
	afterCommit func() error
}
type mailContextConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ExecerContext
	driver.QueryerContext
}
type mailCommitConn struct {
	mailContextConn
	afterCommit func() error
}
type mailCommitTx struct {
	driver.Tx
	afterCommit func() error
}

func (c mailCommitConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &mailCommitConn{conn.(mailContextConn), c.afterCommit}, nil
}
func (c *mailCommitConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.mailContextConn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &mailCommitTx{tx, c.afterCommit}, nil
}
func (t *mailCommitTx) Commit() error {
	if err := t.Tx.Commit(); err != nil {
		return err
	}
	return t.afterCommit()
}

func mailDBWithCommitHook(t *testing.T, d *Database, hook func() error) *sql.DB {
	t.Helper()
	cfg, err := pgx.ParseConfig(fmt.Sprintf("postgres://sa@%s/db?sslmode=disable", d.Server.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	cfg.User = ""
	cfg.RuntimeParams = map[string]string{}
	db := sql.OpenDB(mailCommitConnector{stdlib.GetConnector(*cfg), hook})
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestMailDeliveryUncertainClaimCommitNeverSends(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	lost := errors.New("claim acknowledgement lost")
	db := mailDBWithCommitHook(t, d, func() error { return lost })
	sends := 0
	err := deliveryService(t, db).DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil })
	if !errors.Is(err, repository.ErrCommitOutcomeUnknown) || !errors.Is(err, lost) || !errors.Is(err, service.ErrMailDeliveryUnknown) || sends != 0 {
		t.Fatalf("err=%v sends=%d", err, sends)
	}
	assertAttemptState(t, d.DB, "trip-a", "ATTEMPTING")
	if err := deliveryService(t, d.DB).DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil }); err != nil {
		t.Fatal(err)
	}
	if sends != 0 {
		t.Fatalf("uncertain claim replayed: %d sends", sends)
	}
}

func TestMailDeliveryCanceledAfterClaimReleasesOnlyUnsentAttempt(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a", "PENDING")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var commits atomic.Int32
	db := mailDBWithCommitHook(t, d, func() error {
		if commits.Add(1) == 1 {
			cancel()
		}
		return nil
	})
	sends := 0
	err := deliveryService(t, db).DeliverPending(ctx, "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil })
	if !errors.Is(err, context.Canceled) || sends != 0 {
		t.Fatalf("err=%v sends=%d", err, sends)
	}
	assertAttemptState(t, d.DB, "trip-a", "PENDING")
	if err := deliveryService(t, d.DB).DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil }); err != nil {
		t.Fatal(err)
	}
	if sends != 1 {
		t.Fatalf("unsent pending mail not delivered once: %d sends", sends)
	}
	assertAttemptState(t, d.DB, "trip-a", "ACCEPTED")
}

func TestMailDeliveryLostAcceptedCommitResponsePreservesReceiptAndRecipientIsolation(t *testing.T) {
	d := openTestDB(t)
	seedDeliveryMail(t, d.DB, "trip-a,trip-b", "PENDING")
	lost := errors.New("accepted acknowledgement lost")
	var commits atomic.Int32
	db := mailDBWithCommitHook(t, d, func() error {
		if commits.Add(1) == 2 {
			return lost
		}
		return nil
	})
	sends := 0
	err := deliveryService(t, db).DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil })
	var uncertain *service.MailDeliveryUncertainError
	if !errors.Is(err, repository.ErrCommitOutcomeUnknown) || !errors.Is(err, lost) || !errors.Is(err, service.ErrMailDeliveryUnknown) || !errors.As(err, &uncertain) || uncertain.MailID == 0 || uncertain.AttemptID == "" || sends != 1 {
		t.Fatalf("err=%v sends=%d", err, sends)
	}
	// The service has already attempted markMailUnknown on this failure path.
	assertAttemptState(t, d.DB, "trip-a", "ACCEPTED")
	fresh := deliveryService(t, d.DB)
	if err := fresh.DeliverPending(context.Background(), "alice", "trip-a", func(context.Context, model.Mail) error { sends++; return nil }); err != nil {
		t.Fatal(err)
	}
	if sends != 1 {
		t.Fatalf("accepted recipient was resent: %d", sends)
	}
	otherSends := 0
	if err := fresh.DeliverPending(context.Background(), "bob", "trip-b", func(context.Context, model.Mail) error { otherSends++; return nil }); err != nil {
		t.Fatal(err)
	}
	if otherSends != 1 || sends != 1 {
		t.Fatalf("recipient isolation: alice=%d bob=%d", sends, otherSends)
	}
	assertAttemptState(t, d.DB, "trip-b", "ACCEPTED")
	assertAttemptState(t, d.DB, "trip-a", "ACCEPTED")
}
