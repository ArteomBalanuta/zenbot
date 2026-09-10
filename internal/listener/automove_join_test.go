package listener

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type autoMoveJoinPolicyFake struct {
	destination string
	ok          bool
	calls       int
}

func (f *autoMoveJoinPolicyFake) EligibleReplica(string) (string, bool) {
	f.calls++
	return f.destination, f.ok
}

type autoMoveTripsFake struct {
	trips []string
	err   error
	calls int
}

func (f *autoMoveTripsFake) UserTrips(context.Context) ([]string, error) {
	f.calls++
	return f.trips, f.err
}

type autoMoveMoveFake struct {
	target      common.NickTarget
	destination common.Channel
	calls       int
	err         error
}

func (f *autoMoveMoveFake) BanNick(context.Context, common.NickTarget) error   { return nil }
func (f *autoMoveMoveFake) UnbanHash(context.Context, common.BanHash) error    { return nil }
func (f *autoMoveMoveFake) UnbanAllContext(context.Context) error              { return nil }
func (f *autoMoveMoveFake) LockRoom(context.Context) error                     { return nil }
func (f *autoMoveMoveFake) UnlockRoom(context.Context) error                   { return nil }
func (f *autoMoveMoveFake) EnableCaptcha(context.Context) error                { return nil }
func (f *autoMoveMoveFake) DisableCaptcha(context.Context) error               { return nil }
func (f *autoMoveMoveFake) AuthorizeTrip(context.Context, common.Trip) error   { return nil }
func (f *autoMoveMoveFake) DeauthorizeTrip(context.Context, common.Trip) error { return nil }
func (f *autoMoveMoveFake) MuteNick(context.Context, common.NickTarget) error  { return nil }
func (f *autoMoveMoveFake) UnmuteHash(context.Context, common.BanHash) error   { return nil }
func (f *autoMoveMoveFake) ForceFlair(context.Context, common.NickTarget, common.Flair) error {
	return nil
}
func (f *autoMoveMoveFake) ForceColor(context.Context, common.NickTarget, common.Color) error {
	return nil
}
func (f *autoMoveMoveFake) KickNick(context.Context, common.NickTarget) error { return nil }
func (f *autoMoveMoveFake) KickNickTo(_ context.Context, target common.NickTarget, destination common.Channel) error {
	f.calls++
	f.target = target
	f.destination = destination
	return f.err
}
func (f *autoMoveMoveFake) OverflowNick(context.Context, common.NickTarget) error { return nil }

func TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica(t *testing.T) {
	state := &autoMoveJoinPolicyFake{destination: "dynamic-destination", ok: true}
	trips := &autoMoveTripsFake{trips: []string{"USER-trip"}}
	move := &autoMoveMoveFake{}
	var noticeText, noticeTarget string
	var noticeWhisper bool
	automation := &AutoMoveJoinAutomation{
		State: state,
		Trips: trips,
		Move:  move,
		SendNotice: func(target, text string, whisper bool) (string, error) {
			noticeTarget, noticeText, noticeWhisper = target, text, whisper
			return "", nil
		},
		Channel:   func() string { return "configured-source" },
		IsReplica: func() bool { return true },
	}

	automation.OnJoin(context.Background(), &model.User{Name: "joined", Trip: "USER-trip"})

	if trips.calls != 1 {
		t.Fatalf("UserTrips calls = %d, want 1", trips.calls)
	}
	if noticeTarget != "joined" || noticeWhisper || noticeText != "your trip is authorized to join ?lounge, you will be moved to ?lounge" {
		t.Fatalf("notice = target %q, text %q, whisper %v", noticeTarget, noticeText, noticeWhisper)
	}
	if move.calls != 1 || move.target != common.NickTarget("joined") || move.destination != common.Channel("dynamic-destination") {
		t.Fatalf("kick = calls %d, target %q, destination %q", move.calls, move.target, move.destination)
	}
}

func TestAutoMoveJoinDoesNothingForHost(t *testing.T) {
	state := &autoMoveJoinPolicyFake{destination: "destination", ok: true}
	trips := &autoMoveTripsFake{trips: []string{"trip"}}
	move := &autoMoveMoveFake{}
	notices := 0
	automation := &AutoMoveJoinAutomation{
		State: state, Trips: trips, Move: move,
		SendNotice: func(string, string, bool) (string, error) { notices++; return "", nil },
		Channel:    func() string { return "source" }, IsReplica: func() bool { return false },
	}
	automation.OnJoin(context.Background(), &model.User{Name: "joined", Trip: "trip"})
	if trips.calls != 0 || notices != 0 || move.calls != 0 {
		t.Fatalf("host actions: queries=%d notices=%d kicks=%d", trips.calls, notices, move.calls)
	}
}

func TestAutoMoveJoinGatesDisabledNonSourceNilAndBlankUser(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state *autoMoveJoinPolicyFake
		user  *model.User
	}{
		{"disabled", &autoMoveJoinPolicyFake{}, &model.User{Name: "joined", Trip: "trip"}},
		{"non-source", &autoMoveJoinPolicyFake{destination: "destination", ok: false}, &model.User{Name: "joined", Trip: "trip"}},
		{"nil user", &autoMoveJoinPolicyFake{destination: "destination", ok: true}, nil},
		{"blank name", &autoMoveJoinPolicyFake{destination: "destination", ok: true}, &model.User{Name: " ", Trip: "trip"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trips, move, notices := &autoMoveTripsFake{trips: []string{"trip"}}, &autoMoveMoveFake{}, 0
			a := &AutoMoveJoinAutomation{State: tc.state, Trips: trips, Move: move, SendNotice: func(string, string, bool) (string, error) { notices++; return "", nil }, Channel: func() string { return "source" }, IsReplica: func() bool { return true }}
			a.OnJoin(context.Background(), tc.user)
			if trips.calls != 0 || notices != 0 || move.calls != 0 {
				t.Fatalf("actions: queries=%d notices=%d kicks=%d", trips.calls, notices, move.calls)
			}
		})
	}
}

func TestAutoMoveJoinRequiresExactCaseTripMatch(t *testing.T) {
	trips, move, notices := &autoMoveTripsFake{trips: []string{"Trip"}}, &autoMoveMoveFake{}, 0
	a := &AutoMoveJoinAutomation{State: &autoMoveJoinPolicyFake{destination: "destination", ok: true}, Trips: trips, Move: move, SendNotice: func(string, string, bool) (string, error) { notices++; return "", nil }, Channel: func() string { return "source" }, IsReplica: func() bool { return true }}
	a.OnJoin(context.Background(), &model.User{Name: "joined", Trip: "trip"})
	if notices != 0 || move.calls != 0 {
		t.Fatalf("case mismatch actions: notices=%d kicks=%d", notices, move.calls)
	}
}

func TestAutoMoveJoinTripQueryErrorSendsNeither(t *testing.T) {
	trips, move, notices := &autoMoveTripsFake{trips: []string{"trip"}, err: context.Canceled}, &autoMoveMoveFake{}, 0
	a := &AutoMoveJoinAutomation{State: &autoMoveJoinPolicyFake{destination: "destination", ok: true}, Trips: trips, Move: move, SendNotice: func(string, string, bool) (string, error) { notices++; return "", nil }, Channel: func() string { return "source" }, IsReplica: func() bool { return true }}
	a.OnJoin(context.Background(), &model.User{Name: "joined", Trip: "trip"})
	if trips.calls != 1 || notices != 0 || move.calls != 0 {
		t.Fatalf("query error actions: queries=%d notices=%d kicks=%d", trips.calls, notices, move.calls)
	}
}

func TestAutoMoveJoinPreCancelledContextSendsNeither(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	trips, move, notices := &autoMoveTripsFake{trips: []string{"trip"}}, &autoMoveMoveFake{}, 0
	a := &AutoMoveJoinAutomation{State: &autoMoveJoinPolicyFake{destination: "destination", ok: true}, Trips: trips, Move: move, SendNotice: func(string, string, bool) (string, error) { notices++; return "", nil }, Channel: func() string { return "source" }, IsReplica: func() bool { return true }}
	a.OnJoin(ctx, &model.User{Name: "joined", Trip: "trip"})
	if trips.calls != 0 || notices != 0 || move.calls != 0 {
		t.Fatalf("cancelled actions: queries=%d notices=%d kicks=%d", trips.calls, notices, move.calls)
	}
}

func TestAutoMoveJoinNoticeFailureStillKicks(t *testing.T) {
	move := &autoMoveMoveFake{}
	a := &AutoMoveJoinAutomation{State: &autoMoveJoinPolicyFake{destination: "destination", ok: true}, Trips: &autoMoveTripsFake{trips: []string{"trip"}}, Move: move, SendNotice: func(string, string, bool) (string, error) { return "", errors.New("notice failed") }, Channel: func() string { return "source" }, IsReplica: func() bool { return true }}
	a.OnJoin(context.Background(), &model.User{Name: "joined", Trip: "trip"})
	if move.calls != 1 {
		t.Fatalf("kicks = %d, want 1", move.calls)
	}
}

func TestAutoMoveJoinLogsActionErrorWithoutRetry(t *testing.T) {
	move := &autoMoveMoveFake{err: errors.New("kick failed")}
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	a := &AutoMoveJoinAutomation{State: &autoMoveJoinPolicyFake{destination: "destination", ok: true}, Trips: &autoMoveTripsFake{trips: []string{"trip"}}, Move: move, SendNotice: func(string, string, bool) (string, error) { return "", nil }, Channel: func() string { return "source" }, IsReplica: func() bool { return true }}
	a.OnJoin(context.Background(), &model.User{Name: "joined", Trip: "trip"})
	if move.calls != 1 {
		t.Fatalf("kicks = %d, want no retry", move.calls)
	}
	if !strings.Contains(output.String(), "kick failed") {
		t.Fatalf("action error was not logged: %q", output.String())
	}
}
