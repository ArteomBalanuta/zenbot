package command

import (
	"math/rand/v2"
	"strings"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Kick struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Kick) GetAliases() []string {
	return []string{"kick", "k", "out"}
}

func (u *Kick) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *Kick) NewInstance(engine common.Engine, chatMessage *model.ChatMessage) common.Command {
	return &Kick{
		AccessLevel: model.MODERATOR,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *Kick) Execute() {
	arguments := u.chatMessage.GetArguments()
	if len(arguments) < 2 {
		u.engine.SendChatMessage(u.chatMessage.Name, "Example: "+u.engine.GetPrefix()+"kick merc", u.chatMessage.IsWhisper)
		return
	}
	mode := arguments[1]
	if mode == "-m" {
		for _, raw := range arguments[2:] {
			kickIfPresent(u, raw)
		}
		return
	}
	if mode == "-c" {
		if len(arguments) < 3 {
			return
		}
		for user := range *u.engine.GetActiveUsers() {
			if strings.Contains(user.Name, arguments[2]) {
				u.engine.Kick(user.Name, GetRandomStr(6))
			}
		}
		return
	}
	kickIfPresent(u, mode)
}

func kickIfPresent(u *Kick, raw string) {
	target := strings.TrimPrefix(strings.TrimSpace(raw), "@")
	if target == "" {
		return
	}
	user := u.engine.GetActiveUserByName(target)
	if user != nil {
		u.engine.Kick(target, GetRandomStr(6))
	} else {
		// Saturn intentionally stays silent when a requested nick is absent.
	}
}

func GetRandomStr(n int) string {
	chars := []byte("abcdefgh1234567")
	rstr := make([]byte, n)

	for i := range rstr {
		rstr[i] = chars[rand.IntN(len(chars))]
	}

	return string(rstr)
}
