package core

import (
	"zenbot/internal/config"
	"zenbot/internal/model"
)

type List struct {
	AccessLevel model.Role
	engine      *Engine
	chatMessage *model.ChatMessage
}

func (u *List) GetAliases() []string {
	return []string{"list", "l"}
}

func (u *List) NewInstance(engine *Engine, chatMessage *model.ChatMessage) Command {
	return &List{
		AccessLevel: model.ADMIN,
		engine:      engine,
		chatMessage: chatMessage,
	}
}

func (u *List) GetRole() *model.Role {
	return &u.AccessLevel
}

func (u *List) Execute() {
	var argArr = u.chatMessage.GetArguments()[1:]
	var channel = argArr[0]

	var message = ""
	if u.engine.GetChannel() == channel {
		for u := range u.engine.GetActiveUsers() {
			message += u.Hash + u.Trip + u.Name + "\n"
		}
	} else {
		c := config.SetupConfig()
		e := NewEngine(model.DUMMY, c, nil)
		go e.Start()
		e.HcConnection.Wg.Wait()
		e.Stop()
	}

	u.engine.SendMessage(u.chatMessage.Name, message, u.chatMessage.IsWhisper)
}
