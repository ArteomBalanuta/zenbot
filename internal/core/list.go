package core

import (
	"fmt"
	"time"
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
	var channel = ""
	if len(argArr) > 0 && argArr[0] != "" {
		channel = argArr[0]
	}

	var message = ""
	if u.engine.GetChannel() == channel || channel == "" {
		message = formatActiveUsers(u.engine.GetActiveUsers())
		_, err := u.engine.SendMessage(u.chatMessage.Name, message, u.chatMessage.IsWhisper)
		if err != nil {
			fmt.Println(err)
			return
		}
		return
	}

	c := config.SetupConfig()
	c.Channel = channel
	callbackChan := make(chan string, 1)

	zombie := NewEngine(model.ZOMBIE, c, nil)
	zombie.OnlineSetListener = NewOnlineSetListener(zombie, func(z *Engine) {
		callbackChan <- formatActiveUsers(z.GetActiveUsers())
	})

	go zombie.Start()

	select {
	case activeUsersFmt := <-callbackChan:
		message = activeUsersFmt
	case <-time.After(30 * time.Second):
		fmt.Println("ERROR: Callback timeout")
	}

	close(callbackChan)

	zombie.Stop()
	zombie.HcConnection.Wg.Wait()

	_, _ = u.engine.SendMessage(u.chatMessage.Name, message, u.chatMessage.IsWhisper)
}

func formatActiveUsers(users map[*model.User]struct{}) string {
	var message = ""
	for u := range users {
		var trip = u.Trip
		if trip == "" {
			trip = "------"
		}
		message += "\n" + u.Hash + " | " + trip + " | " + u.Name + "\n"
	}
	return message
}
