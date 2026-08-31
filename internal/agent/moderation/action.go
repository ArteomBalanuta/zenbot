package moderation

type Action string

const (
	Warn      Action = "warning"
	Captcha   Action = "captcha"
	Mute      Action = "mute"
	Kick      Action = "kick"
	ShadowBan Action = "shadow-ban"
)

type Decision struct {
	Action            Action
	Principal, Reason string
}
type ActionExecutor interface{ Execute(Decision) error }
