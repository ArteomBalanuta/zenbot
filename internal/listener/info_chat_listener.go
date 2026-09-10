package listener

import (
	"context"
	"encoding/json"
	"log"
	"zenbot/internal/common"
	"zenbot/internal/listener/info"
	"zenbot/internal/model"
)

type InfoChatListener struct {
	engine common.Engine
	chain  *info.Chain
}

func NewInfoChatListener(e common.Engine) *InfoChatListener {
	return &InfoChatListener{engine: e, chain: info.DefaultChain()}
}
func SliceUpTo(s string, n int) string {
	if n < 0 || n > len([]rune(s)) {
		return s
	}
	return string([]rune(s)[:n])
}
func SliceDownTo(s string, n int) string {
	r := []rune(s)
	if n < 0 || n+1 > len(r) {
		return ""
	}
	return string(r[n+1:])
}
func (u *InfoChatListener) Notify(text string) {
	var m model.InfoMessage
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		log.Printf("malformed info payload: %v", err)
		return
	}
	if err := u.chain.Process(context.Background(), &m, u.engine); err != nil {
		log.Printf("info listener stopped: %v", err)
	}
}
