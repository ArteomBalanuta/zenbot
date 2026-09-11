package common

import "context"

// Distinct input types prevent callers from accidentally placing a hash, trip,
// or channel in a raw moderation payload's nick field. NickTarget is an
// already-resolved canonical source nickname; raw user operands must be
// normalized and resolved before constructing it.
type NickTarget string
type BanHash string
type Trip string
type Flair string
type Color string
type Channel string

// ModerationOperations is the narrow context-aware raw protocol boundary used
// by moderator command slices. It does not define command policy or output.
type ModerationOperations interface {
	BanNick(context.Context, NickTarget) error
	UnbanHash(context.Context, BanHash) error
	UnbanAllContext(context.Context) error
	LockRoom(context.Context) error
	UnlockRoom(context.Context) error
	EnableCaptcha(context.Context) error
	DisableCaptcha(context.Context) error
	AuthorizeTrip(context.Context, Trip) error
	DeauthorizeTrip(context.Context, Trip) error
	MuteNick(context.Context, NickTarget) error
	UnmuteHash(context.Context, BanHash) error
	ForceFlair(context.Context, NickTarget, Flair) error
	ForceColor(context.Context, NickTarget, Color) error
	KickNick(context.Context, NickTarget) error
	KickNickTo(context.Context, NickTarget, Channel) error
	OverflowNick(context.Context, NickTarget) error
}
