package listener

import (
	"context"
	"encoding/json"
	"log"
	"zenbot/internal/common"
	"zenbot/internal/listener/message"
	"zenbot/internal/model"
)

type UserChatListener struct {
	engine common.Engine
	chain  *message.Chain
}

func NewUserChatListener(e common.Engine) *UserChatListener {
	return NewUserChatListenerWithChain(e, nil)
}
func NewUserChatListenerWithChain(e common.Engine, chain *message.Chain) *UserChatListener {
	if chain == nil {
		chain = message.DefaultChain()
	}
	return &UserChatListener{engine: e, chain: chain}
}
func (u *UserChatListener) Notify(text string) {
	var m model.ChatMessage
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		log.Printf("malformed chat payload: %v", err)
		return
	}
	m.IsWhisper = m.IsWhisper || m.Whisper || m.Type == "whisper"
	if err := u.chain.Process(context.Background(), &m, u.engine); err != nil {
		log.Printf("chat listener stopped: %v", err)
	}
}
