package command

import (
	"context"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

func TestRemoveAndUnshadowBanAliasesResolveMentionedNamesInPersistence(t *testing.T) {
	d := h2fixture.Open(t, "nickname-persistence-aliases")
	e := &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{GroupB: d}, ShadowBans: &service.ShadowBanService{Repo: d}}}
	for _, alias := range []string{"del", "delete", "remove", "unshadowban", "shadowmercy", "unblock"} {
		for _, raw := range []string{"merc", "@merc"} {
			t.Run(alias+raw, func(t *testing.T) {
				definition, _ := commandDefinitionFor(alias)
				if definition.Canonical == "remove" {
					for _, q := range []string{`INSERT INTO names(name,created_on) VALUES('merc',1)`, `INSERT INTO trips(type,trip,created_on) VALUES('USER','trip',1)`, `INSERT INTO trip_names(name_id,trip_id) SELECT n.id,t.id FROM names n,trips t`} {
						if _, err := d.DB.Exec(q); err != nil {
							t.Fatal(err)
						}
					}
				} else if err := d.PersistShadowBanRecord(context.Background(), repository.ShadowBanRecord{Name: "merc", Trip: "trip", Hash: "hash"}); err != nil {
					t.Fatal(err)
				}
				status, err := definition.New(e, &model.ChatMessage{Name: "mod", Text: "*" + alias + " " + raw}).Execute(context.Background())
				if err != nil || status != model.SUCCESSFUL {
					t.Fatalf("status=%v err=%v", status, err)
				}
				q := `SELECT COUNT(*) FROM banned_users`
				if definition.Canonical == "remove" {
					q = `SELECT COUNT(*) FROM names`
				}
				var count int
				if err := d.DB.QueryRow(q).Scan(&count); err != nil || count != 0 {
					t.Fatalf("remaining rows=%d err=%v", count, err)
				}
			})
		}
	}
}

func TestMailAliasesResolveBothNicknameFormsWithoutChangingBody(t *testing.T) {
	e, db := openMailGroupCParityEngine(t)
	seedMailGroupCCommandRecipient(t, db)
	for _, alias := range []string{"mail", "msg", "send"} {
		for _, raw := range []string{"merc", "@merc"} {
			if status := executeMailGroupCCommand(t, e, alias, " "+raw+" literal \\n real\nline"); status != model.SUCCESSFUL {
				t.Fatalf("alias=%s raw=%s status=%v", alias, raw, status)
			}
			var receiver, body, encoding string
			if err := db.QueryRow(`SELECT receiver,message,text_encoding FROM mail ORDER BY id DESC LIMIT 1`).Scan(&receiver, &body, &encoding); err != nil {
				t.Fatal(err)
			}
			if receiver != "trip-a" || body != "literal \\n real\nline " || encoding != "PLAIN" {
				t.Fatalf("receiver=%q body=%q encoding=%q", receiver, body, encoding)
			}
		}
	}
}
