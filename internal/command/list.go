package command

import (
	"fmt"
	"sort"
	"time"

	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/factory"
	"zenbot/internal/listener"
	"zenbot/internal/model"
)

type List struct {
	AccessLevel model.Role
	engine      common.Engine
	chatMessage *model.ChatMessage
}

func (u *List) GetAliases() []string {
	return []string{"list"}
}

func (u *List) NewInstance(engine common.Engine, chatMessage *model.ChatMessage) common.Command {
	return &List{AccessLevel: model.USER, engine: engine, chatMessage: chatMessage}
}

func (u *List) GetRole() *model.Role { return &u.AccessLevel }

func (u *List) Execute() {
	args := u.chatMessage.GetArguments()[1:]
	if len(args) == 0 {
		message := formatActiveUsers(*u.engine.GetActiveUsers())
		_, _ = u.engine.SendChatMessage(u.chatMessage.Name, message, u.chatMessage.IsWhisper)
		_, _ = u.engine.SendChatMessage(u.chatMessage.Name, "Example: "+u.engine.GetPrefix()+"list programming", u.chatMessage.IsWhisper)
		return
	}

	channel := args[0]
	if channel == "" || u.engine.GetChannel() == channel {
		_, _ = u.engine.SendChatMessage(u.chatMessage.Name, formatActiveUsers(*u.engine.GetActiveUsers()), u.chatMessage.IsWhisper)
		return
	}

	callbackChan := make(chan string, 1)
	c := config.SetupConfig()
	c.Channel = channel
	zombie := factory.NewEngine(model.ZOMBIE, c, nil)
	onlineSetListener := listener.NewOnlineSetListener(zombie, func(z common.Engine) {
		callbackChan <- formatActiveUsers(*z.GetActiveUsers())
	})
	zombie.SetOnlineSetListener(onlineSetListener)
	go zombie.Start()

	message := ""
	select {
	case message = <-callbackChan:
	case <-time.After(30 * time.Second):
		fmt.Println("ERROR: Callback timeout")
	}
	close(callbackChan)
	zombie.Stop()
	zombie.WaitConnectionWgDone()
	_, _ = u.engine.SendChatMessage(u.chatMessage.Name, message, u.chatMessage.IsWhisper)
}

func formatActiveUsers(users map[*model.User]struct{}) string {
	ordered := make([]*model.User, 0, len(users))
	for user := range users {
		ordered = append(ordered, user)
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Hash < ordered[j].Hash })

	message := "\nUsers online: \n"
	for _, user := range ordered {
		trip := user.Trip
		if trip == "" {
			trip = "------"
		}
		message += user.Hash + " - " + trip + " - " + user.Name + "\n"
	}
	return message + "\n"
}
