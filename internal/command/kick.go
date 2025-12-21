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
	for user := range *u.engine.GetActiveUsers() {
		if user.Name == target {
			u.engine.Kick(target, channel)
			break
		}
	}
}

func GetRandomStr(n int) string {
	chars := []byte("abcdefgh")
	rstr := make([]byte, 6)

	for i := range rstr {
		rstr[i] = chars[rand.IntN(len(chars))]
	}

	return string(rstr)
}
