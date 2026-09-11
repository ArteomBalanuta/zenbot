package core

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/transport"
)

func TestRawModerationOperationsEmitExactSaturnPayloads(t *testing.T) {
	engine := &EngineImpl{OutMessageQueue: make(chan string, 16)}
	for _, tc := range []struct {
		name string
		run  func(context.Context) error
		want string
	}{
		{"ban preserves ordinary nick", func(ctx context.Context) error { return engine.BanNick(ctx, common.NickTarget("raider")) }, `{"cmd":"ban","nick":"raider"}`},
		{"ban JSON-escapes canonical nick", func(ctx context.Context) error {
			return engine.BanNick(ctx, common.NickTarget(`@raid"er\path`))
		}, `{"cmd":"ban","nick":"@raid\"er\\path"}`},
		{"unban preserves hash", func(ctx context.Context) error { return engine.UnbanHash(ctx, common.BanHash("hash value")) }, `{"cmd":"unban","hash":"hash value"}`},
		{"unban JSON-escapes raw hash", func(ctx context.Context) error {
			return engine.UnbanHash(ctx, common.BanHash("hash\"line\nbreak"))
		}, `{"cmd":"unban","hash":"hash\"line\nbreak"}`},
		{"unban all", func(ctx context.Context) error { return engine.UnbanAllContext(ctx) }, `{"cmd":"unbanall"}`},
		{"lock", func(ctx context.Context) error { return engine.LockRoom(ctx) }, `{"cmd":"lockroom"}`},
		{"unlock", func(ctx context.Context) error { return engine.UnlockRoom(ctx) }, `{"cmd":"unlockroom"}`},
		{"enable captcha", func(ctx context.Context) error { return engine.EnableCaptcha(ctx) }, `{"cmd":"enablecaptcha"}`},
		{"disable captcha", func(ctx context.Context) error { return engine.DisableCaptcha(ctx) }, `{"cmd":"disablecaptcha"}`},
		{"authorize trip", func(ctx context.Context) error { return engine.AuthorizeTrip(ctx, common.Trip("trip\"quoted")) }, `{"cmd":"authtrip","trip":"trip\"quoted"}`},
		{"deauthorize trip", func(ctx context.Context) error { return engine.DeauthorizeTrip(ctx, common.Trip("trip")) }, `{"cmd":"deauthtrip","trip":"trip"}`},
		{"mute preserves marker nick", func(ctx context.Context) error { return engine.MuteNick(ctx, common.NickTarget("@raider")) }, `{"cmd":"mute","nick":"@raider"}`},
		{"unmute preserves hash", func(ctx context.Context) error { return engine.UnmuteHash(ctx, common.BanHash("hash\nvalue")) }, `{"cmd":"unmute","hash":"hash\nvalue"}`},
		{"force flair preserves Unicode nick", func(ctx context.Context) error {
			return engine.ForceFlair(ctx, common.NickTarget("Ålice"), common.Flair("badge\"x"))
		}, `{"cmd":"forceflair","nick":"Ålice","flair":"badge\"x"}`},
		{"force color preserves nick case", func(ctx context.Context) error {
			return engine.ForceColor(ctx, common.NickTarget("Merc"), common.Color("#00ff00"))
		}, `{"cmd":"forcecolor","nick":"Merc","color":"#00ff00"}`},
		{"kick preserves marker nick", func(ctx context.Context) error { return engine.KickNick(ctx, common.NickTarget("@raider")) }, `{"cmd":"kick","nick":"@raider"}`},
		{"kick to preserves marker nick", func(ctx context.Context) error {
			return engine.KickNickTo(ctx, common.NickTarget("@raider"), common.Channel("lobby"))
		}, `{"cmd":"kick","nick":"@raider","to":"lobby"}`},
		{"overflow preserves Unicode nick", func(ctx context.Context) error { return engine.OverflowNick(ctx, common.NickTarget("玩家")) }, `{"cmd":"overflow","nick":"玩家"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := <-engine.OutMessageQueue; got != tc.want {
				t.Fatalf("payload = %q, want %q", got, tc.want)
			}
		})
	}
	if got := len(engine.OutMessageQueue); got != 0 {
		t.Fatalf("unexpected outbound payloads: %d", got)
	}
}

func TestRawModerationOperationsRejectBlankNickWithoutOutput(t *testing.T) {
	engine := &EngineImpl{OutMessageQueue: make(chan string, 6)}
	for _, operation := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"ban", func(ctx context.Context) error { return engine.BanNick(ctx, common.NickTarget("   ")) }},
		{"mute", func(ctx context.Context) error { return engine.MuteNick(ctx, common.NickTarget("")) }},
		{"flair", func(ctx context.Context) error {
			return engine.ForceFlair(ctx, common.NickTarget("\t"), common.Flair("badge"))
		}},
		{"color", func(ctx context.Context) error {
			return engine.ForceColor(ctx, common.NickTarget(""), common.Color("#fff"))
		}},
		{"kick", func(ctx context.Context) error { return engine.KickNick(ctx, common.NickTarget("")) }},
		{"overflow", func(ctx context.Context) error { return engine.OverflowNick(ctx, common.NickTarget("")) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(context.Background()); err == nil {
				t.Fatal("blank nick accepted")
			}
		})
	}
	if got := len(engine.OutMessageQueue); got != 0 {
		t.Fatalf("blank nick emitted %d payloads", got)
	}
}

func TestRawModerationOperationsSendNothingWhenCancelled(t *testing.T) {
	engine := &EngineImpl{OutMessageQueue: make(chan string, 16)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, operation := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"ban", func(ctx context.Context) error { return engine.BanNick(ctx, common.NickTarget("raider")) }},
		{"unban", func(ctx context.Context) error { return engine.UnbanHash(ctx, common.BanHash("hash")) }},
		{"unban all", engine.UnbanAllContext},
		{"lock", engine.LockRoom},
		{"unlock", engine.UnlockRoom},
		{"enable captcha", engine.EnableCaptcha},
		{"disable captcha", engine.DisableCaptcha},
		{"authorize", func(ctx context.Context) error { return engine.AuthorizeTrip(ctx, common.Trip("trip")) }},
		{"deauthorize", func(ctx context.Context) error { return engine.DeauthorizeTrip(ctx, common.Trip("trip")) }},
		{"mute", func(ctx context.Context) error { return engine.MuteNick(ctx, common.NickTarget("raider")) }},
		{"unmute", func(ctx context.Context) error { return engine.UnmuteHash(ctx, common.BanHash("hash")) }},
		{"flair", func(ctx context.Context) error {
			return engine.ForceFlair(ctx, common.NickTarget("raider"), common.Flair("badge"))
		}},
		{"color", func(ctx context.Context) error {
			return engine.ForceColor(ctx, common.NickTarget("raider"), common.Color("#fff"))
		}},
		{"kick", func(ctx context.Context) error { return engine.KickNick(ctx, common.NickTarget("raider")) }},
		{"kick to", func(ctx context.Context) error {
			return engine.KickNickTo(ctx, common.NickTarget("raider"), common.Channel("lobby"))
		}},
		{"overflow", func(ctx context.Context) error { return engine.OverflowNick(ctx, common.NickTarget("raider")) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if got := len(engine.OutMessageQueue); got != 0 {
		t.Fatalf("cancelled operations emitted %d payloads", got)
	}
}

type moderationFailingTransport struct{ calls int }

func (t *moderationFailingTransport) Start(context.Context) error               { return nil }
func (t *moderationFailingTransport) Messages() <-chan transport.InboundMessage { return nil }
func (t *moderationFailingTransport) Errors() <-chan error                      { return nil }
func (t *moderationFailingTransport) Connected() bool                           { return true }
func (t *moderationFailingTransport) SendText(context.Context, string) error {
	t.calls++
	return errors.New("send failed")
}
func (t *moderationFailingTransport) SendRaw(context.Context, []byte) error { return nil }
func (t *moderationFailingTransport) Close(context.Context) error           { return nil }

func TestRawModerationOperationReturnsTransportErrorWithoutFallbackOutput(t *testing.T) {
	transport := &moderationFailingTransport{}
	engine := &EngineImpl{Transport: transport, OutMessageQueue: make(chan string, 1)}
	if err := engine.BanNick(context.Background(), common.NickTarget("raider")); err == nil {
		t.Fatal("transport error was swallowed")
	}
	if transport.calls != 1 {
		t.Fatalf("send calls = %d", transport.calls)
	}
	if got := len(engine.OutMessageQueue); got != 0 {
		t.Fatalf("failed operation emitted fallback output: %d", got)
	}
}

var _ common.ModerationOperations = (*EngineImpl)(nil)
