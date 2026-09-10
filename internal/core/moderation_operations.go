package core

import (
	"context"
	"encoding/json"

	"zenbot/internal/common"
	"zenbot/internal/util"
)

// moderationPayload is deliberately closed over the Saturn ModServiceImpl
// protocol vocabulary. Omitempty preserves each command's exact observed shape.
type moderationPayload struct {
	Command string `json:"cmd"`
	Nick    string `json:"nick,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Trip    string `json:"trip,omitempty"`
	Flair   string `json:"flair,omitempty"`
	Color   string `json:"color,omitempty"`
	To      string `json:"to,omitempty"`
}

func (e *EngineImpl) BanNick(ctx context.Context, target common.NickTarget) error {
	nick, err := normalizeModerationNick(target)
	if err != nil {
		return err
	}
	return e.sendRawModeration(ctx, moderationPayload{Command: "ban", Nick: nick})
}

func (e *EngineImpl) UnbanHash(ctx context.Context, hash common.BanHash) error {
	return e.sendRawModeration(ctx, moderationPayload{Command: "unban", Hash: string(hash)})
}

func (e *EngineImpl) UnbanAllContext(ctx context.Context) error {
	return e.sendRawModeration(ctx, moderationPayload{Command: "unbanall"})
}

func (e *EngineImpl) LockRoom(ctx context.Context) error {
	return e.sendRawModeration(ctx, moderationPayload{Command: "lockroom"})
}

func (e *EngineImpl) UnlockRoom(ctx context.Context) error {
	return e.sendRawModeration(ctx, moderationPayload{Command: "unlockroom"})
}

func (e *EngineImpl) EnableCaptcha(ctx context.Context) error {
	return e.sendRawModeration(ctx, moderationPayload{Command: "enablecaptcha"})
}

func (e *EngineImpl) DisableCaptcha(ctx context.Context) error {
	return e.sendRawModeration(ctx, moderationPayload{Command: "disablecaptcha"})
}

func (e *EngineImpl) AuthorizeTrip(ctx context.Context, trip common.Trip) error {
	return e.sendRawModeration(ctx, moderationPayload{Command: "authtrip", Trip: string(trip)})
}

func (e *EngineImpl) DeauthorizeTrip(ctx context.Context, trip common.Trip) error {
	return e.sendRawModeration(ctx, moderationPayload{Command: "deauthtrip", Trip: string(trip)})
}

func (e *EngineImpl) MuteNick(ctx context.Context, target common.NickTarget) error {
	nick, err := normalizeModerationNick(target)
	if err != nil {
		return err
	}
	return e.sendRawModeration(ctx, moderationPayload{Command: "mute", Nick: nick})
}

func (e *EngineImpl) UnmuteHash(ctx context.Context, hash common.BanHash) error {
	return e.sendRawModeration(ctx, moderationPayload{Command: "unmute", Hash: string(hash)})
}

func (e *EngineImpl) ForceFlair(ctx context.Context, target common.NickTarget, flair common.Flair) error {
	nick, err := normalizeModerationNick(target)
	if err != nil {
		return err
	}
	return e.sendRawModeration(ctx, moderationPayload{Command: "forceflair", Nick: nick, Flair: string(flair)})
}

func (e *EngineImpl) ForceColor(ctx context.Context, target common.NickTarget, color common.Color) error {
	nick, err := normalizeModerationNick(target)
	if err != nil {
		return err
	}
	return e.sendRawModeration(ctx, moderationPayload{Command: "forcecolor", Nick: nick, Color: string(color)})
}

func (e *EngineImpl) KickNick(ctx context.Context, target common.NickTarget) error {
	nick, err := normalizeModerationNick(target)
	if err != nil {
		return err
	}
	return e.sendRawModeration(ctx, moderationPayload{Command: "kick", Nick: nick})
}

func (e *EngineImpl) KickNickTo(ctx context.Context, target common.NickTarget, channel common.Channel) error {
	nick, err := normalizeModerationNick(target)
	if err != nil {
		return err
	}
	return e.sendRawModeration(ctx, moderationPayload{Command: "kick", Nick: nick, To: string(channel)})
}

func (e *EngineImpl) OverflowNick(ctx context.Context, target common.NickTarget) error {
	nick, err := normalizeModerationNick(target)
	if err != nil {
		return err
	}
	return e.sendRawModeration(ctx, moderationPayload{Command: "overflow", Nick: nick})
}

func normalizeModerationNick(target common.NickTarget) (string, error) {
	raw := string(target)
	return util.NormalizeNickTarget(&raw)
}

func (e *EngineImpl) sendRawModeration(ctx context.Context, payload moderationPayload) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return e.sendOutboundContext(ctx, string(encoded))
}
