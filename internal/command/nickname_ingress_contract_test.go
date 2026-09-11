package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

func TestRegisterAliasesNormalizeRawNickOnceAndPreserveTrip(t *testing.T) {
	for _, alias := range []string{"reg", "register"} {
		for _, tc := range []struct{ raw, want string }{{"merc", "merc"}, {"@merc", "merc"}, {"@@merc", "@merc"}} {
			t.Run(alias+tc.raw, func(t *testing.T) {
				ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
				e := newIdentityEngine(ids, &authFake{})
				d, _ := commandDefinitionFor(alias)
				status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "*" + alias + " " + tc.raw + " @Trip"}).Execute(context.Background())
				if err != nil || status != model.SUCCESSFUL || len(ids.registered) != 1 || ids.registered[0] != tc.want+":@Trip" {
					t.Fatalf("status=%v err=%v registered=%q", status, err, ids.registered)
				}
			})
		}
	}
}

func TestAllOnlineNicknameAliasesReachCanonicalWireTarget(t *testing.T) {
	for _, tc := range []struct {
		aliases         []string
		suffix, command string
	}{
		{[]string{"ban"}, "", "ban"}, {[]string{"kick", "k", "out"}, "", "kick"},
		{[]string{"mute", "dumb"}, "", "mute"}, {[]string{"color"}, " 00ff00", "forcecolor"},
		{[]string{"flair"}, " trusted", "forceflair"}, {[]string{"overflow", "shoot", "love", "hug", "kiss"}, "", "overflow"},
	} {
		for _, alias := range tc.aliases {
			for _, raw := range []string{"merc", "@merc"} {
				t.Run(alias+raw, func(t *testing.T) {
					e := &core.EngineImpl{Prefix: "*", OutMessageQueue: make(chan string, 4)}
					e.ReplaceActiveUsers([]*model.User{{Name: "MeRc", Hash: "hash", Trip: "Trip"}})
					d, _ := commandDefinitionFor(alias)
					status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "*" + alias + " " + raw + tc.suffix}).Execute(context.Background())
					if err != nil || status != model.SUCCESSFUL {
						t.Fatalf("status=%v err=%v", status, err)
					}
					select {
					case frame := <-e.OutMessageQueue:
						var got struct {
							Cmd  string `json:"cmd"`
							Nick string `json:"nick"`
						}
						if err := json.Unmarshal([]byte(frame), &got); err != nil {
							t.Fatal(err)
						}
						if got.Cmd != tc.command || got.Nick != "MeRc" {
							t.Fatalf("wire=%s", frame)
						}
					default:
						t.Fatal("missing moderation frame")
					}
				})
			}
		}
	}
}

func TestInfoAndShadowBanAliasesNormalizeRawNickname(t *testing.T) {
	for _, alias := range []string{"info", "i", "whois", "who", "shadowban", "sban"} {
		for _, raw := range []string{"merc", "@merc"} {
			t.Run(alias+raw, func(t *testing.T) {
				repo := &shadowBanRepositoryStub{}
				e := &core.EngineImpl{Prefix: "*", OutMessageQueue: make(chan string, 4), Services: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: repo}}}
				e.ReplaceActiveUsers([]*model.User{{Name: "merc", Hash: "observed-hash", Trip: "observed-trip"}})
				d, _ := commandDefinitionFor(alias)
				status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "*" + alias + " " + raw}).Execute(context.Background())
				if err != nil || status != model.SUCCESSFUL {
					t.Fatalf("status=%v err=%v", status, err)
				}
				if d.Canonical == "shadowban" {
					if len(repo.persisted) != 1 || repo.persisted[0].Name != "merc" {
						t.Fatalf("records=%+v", repo.persisted)
					}
				} else {
					select {
					case frame := <-e.OutMessageQueue:
						if !strings.Contains(frame, "observed-trip") || !strings.Contains(frame, "observed-hash") {
							t.Fatalf("reply=%s", frame)
						}
					default:
						t.Fatal("missing info reply")
					}
				}
			})
		}
	}
}

func TestMoveAliasesPropagateNormalizedNickAndLiteralRooms(t *testing.T) {
	for _, alias := range []string{"move", "recover", "heal", "resurrect"} {
		for _, raw := range []string{"merc", "@merc"} {
			e := &credentialedResurrectFallbackStub{resurrectFallbackStub: &resurrectFallbackStub{commandEngineStub: &commandEngineStub{}}}
			d, _ := commandDefinitionFor(alias)
			status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "*" + alias + " " + raw + " source@room dest@room"}).Execute(context.Background())
			if err != nil || status != model.SUCCESSFUL || len(e.moves) != 1 || e.moves[0].target != "merc" || e.moves[0].from != "source@room" || e.moves[0].to != "dest@room" || len(e.credentialedRequests) != 1 || e.credentialedRequests[0].TargetChannel != "source@room" {
				t.Fatalf("alias=%s raw=%s status=%v err=%v moves=%+v requests=%+v", alias, raw, status, err, e.moves, e.credentialedRequests)
			}
		}
	}
}

func TestNukePreservesRoomAtSigns(t *testing.T) {
	for _, room := range []string{"room@host", "@room", "?room@host"} {
		e := &nukeSubmitterStub{commandEngineStub: &commandEngineStub{}}
		d, _ := commandDefinitionFor("nuke")
		status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "*nuke " + room}).Execute(context.Background())
		want := room
		if room == "?room@host" {
			want = "room@host"
		}
		if err != nil || status != model.SUCCESSFUL || len(e.requests) != 1 || e.requests[0].TargetChannel != want {
			t.Fatalf("room=%q status=%v err=%v requests=%+v", room, status, err, e.requests)
		}
	}
}
