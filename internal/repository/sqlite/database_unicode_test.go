package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

func TestSQLiteUnicodeNicknameLookupAndMail(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	for _, name := range []string{"ÉLODIE", "ИВАН"} {
		if err := d.Register(ctx, name, "Trip-"+name, model.USER); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, trip string }{{"élodie", "Trip-ÉLODIE"}, {"иван", "Trip-ИВАН"}} {
		found, err := d.IsNameRegistered(ctx, tc.name)
		if err != nil || !found {
			t.Errorf("registered %q=%v: %v", tc.name, found, err)
		}
		svc := service.MailService{DB: d.DB}
		got, err := svc.QueueResolved(ctx, "hello", "sender", tc.name, true)
		if err != nil || got != tc.trip {
			t.Errorf("mail %q resolved %q: %v", tc.name, got, err)
		}
	}
}

func TestSQLiteUnicodeLowerEveryConnectionAndValueTypes(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	var connections []*sql.Conn
	defer func() {
		for _, conn := range connections {
			conn.Close()
		}
	}()
	for i := 0; i < 4; i++ {
		conn, err := d.DB.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
		var folded string
		if err := conn.QueryRowContext(ctx, `SELECT lower('ÉLODIE ИВАН')`).Scan(&folded); err != nil || folded != "élodie иван" {
			t.Errorf("folded %q: %v", folded, err)
		}
	}
	for _, tc := range []struct{ expression, want string }{{`lower(123)`, "123"}, {`lower(1.0)`, "1.0"}, {`lower(x'4142')`, "ab"}, {`typeof(lower(NULL))`, "null"}} {
		var got string
		if err := d.DB.QueryRowContext(ctx, `SELECT `+tc.expression).Scan(&got); err != nil || got != tc.want {
			t.Errorf("%s=%q want %q: %v", tc.expression, got, tc.want, err)
		}
	}
}
