package command

import (
	"math/rand/v2"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type Kick struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *Kick) GetAliases() []string {
	return []string{"kick", "k"}
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
	target := u.chatMessage.GetArguments()[1:][0]
	channel := GetRandomStr(6)

	user := u.engine.GetActiveUserByName(target)
	if user != nil {
		u.engine.Kick(target, channel)
		u.engine.SendChatMessage(u.chatMessage.Name, " user has been kicked", u.chatMessage.IsWhisper)
	} else {
		u.engine.SendChatMessage(u.chatMessage.Name, " user not found", u.chatMessage.IsWhisper)
	}
}

func GetRandomStr(n int) string {
	chars := []byte("abcdefgh1234567")
	rstr := make([]byte, 6)

	for i := range rstr {
		rstr[i] = chars[rand.IntN(len(chars))]
	}

	return string(rstr)
}
