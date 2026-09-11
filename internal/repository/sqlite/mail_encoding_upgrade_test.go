package sqlite

import (
	"context"
	"testing"
	"zenbot/internal/service"
)

func TestBootstrapTagsLegacyMailWithoutRewritingText(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.DB.Exec(`ALTER TABLE mail DROP COLUMN text_encoding`); err != nil {
		t.Fatal(err)
	}
	const legacy = `actual\n literal \\n quote \"`
	if _, err := d.DB.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES('sender','trip',?,'PENDING',1,'true')`, legacy); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := bootstrap(context.Background(), d.DB); err != nil {
			t.Fatal(err)
		}
		var stored, encoding string
		if err := d.DB.QueryRow(`SELECT message,text_encoding FROM mail`).Scan(&stored, &encoding); err != nil {
			t.Fatal(err)
		}
		if stored != legacy || encoding != "JSON_STRING" {
			t.Fatalf("stored=%q encoding=%q", stored, encoding)
		}
		got, err := (&service.MailService{DB: d.DB}).Pending(context.Background(), "merc", "trip")
		if err != nil || len(got) != 1 || got[0].Message != "actual\n literal \\n quote \"" {
			t.Fatalf("pending=%+v err=%v", got, err)
		}
	}
}
