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
		for u := range u.engine.GetActiveUsers() {
			message += "\n" + u.Hash + " | " + u.Trip + " | " + u.Name + "\n"
		}
	} else {
		c := config.SetupConfig()
		c.Channel = channel

		zombie := NewEngine(model.ZOMBIE, c, nil)

		callbackChan := make(chan string, 1)

		// setting callback!
		zombie.OnlineSetListener = NewOnlineSetListener(zombie, func(z *Engine) {
			fmt.Println("########qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq")
			var message = ""
			for u := range z.GetActiveUsers() {
				message += "\n" + u.Hash + " | " + u.Trip + " | " + u.Name + "\n"
			}

			callbackChan <- message
		})

		go zombie.Start()

		// Wait for callback result
		select {
		case message := <-callbackChan:
			fmt.Println("Callback executed..", message)
		case <-time.After(30 * time.Second):
			fmt.Println("Callback timeout")
		}

		close(callbackChan)

		// IMPORTANT: Stop engine BEFORE waiting for WaitGroup
		zombie.Stop()

		zombie.HcConnection.Wg.Wait()

	}

	u.engine.SendMessage(u.chatMessage.Name, message, u.chatMessage.IsWhisper)
}
