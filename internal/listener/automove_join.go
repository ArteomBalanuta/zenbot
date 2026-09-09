package listener

import (
	"context"
	"log"
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

const autoMoveJoinNotice = "your trip is authorized to join ?lounge, you will be moved to ?lounge"

// AutoMoveJoinAutomation applies automove eligibility to one already-parsed join.
type AutoMoveJoinAutomation struct {
	State      common.AutoMoveJoinPolicy
	Trips      repository.AutoMoveTripRepository
	Move       common.ModerationOperations
	SendNotice func(string, string, bool) (string, error)
	Channel    func() string
	IsReplica  func() bool
}

func (a *AutoMoveJoinAutomation) OnJoin(ctx context.Context, user *model.User) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil || user == nil || strings.TrimSpace(user.Name) == "" {
		return
	}
	if !a.IsReplica() {
		return
	}
	destination, ok := a.State.EligibleReplica(a.Channel())
	if !ok {
		return
	}
	trips, err := a.Trips.UserTrips(ctx)
	if err != nil {
		return
	}
	for _, trip := range trips {
		if trip == user.Trip {
			if _, err := a.SendNotice(user.Name, autoMoveJoinNotice, false); err != nil {
				log.Printf("automove notice failed for %q: %v", user.Name, err)
			}
			if err := a.Move.KickNickTo(ctx, common.NickTarget(user.Name), common.Channel(destination)); err != nil {
				log.Printf("automove kick failed for %q: %v", user.Name, err)
			}
			return
		}
	}
}
