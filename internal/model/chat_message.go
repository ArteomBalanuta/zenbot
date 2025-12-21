package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

type InfoMessage struct {
	Size    string
	Info    string      `json:"info"`
	From    interface{} `json:"from"`
	Name    string      `json:"nick"`
	Trip    string      `json:"trip"`
	Time    uint64      `json:"time"`
	Channel interface{} `json:"channel"`
	Text    string      `json:"text"`
	Mod     bool        `json:"mod"`
	Flair   interface{} `json:"flair"`
	Color   string      `json:"color"`
}

type ChatMessage struct {
	IsWhisper bool
	Size      string
	Cmd       string      `json:"cmd"`
	Name      string      `json:"nick"`
	Trip      string      `json:"trip"`
	Hash      string      `json:"hash"`
	Time      uint64      `json:"time"`
	Channel   string      `json:"channel"`
	Text      string      `json:"text"`
	Mod       bool        `json:"mod"`
	Flair     interface{} `json:"flair"`
	Color     string      `json:"color"`
}

func FromJson[T ChatMessage | InfoMessage](jsonText string) *T {
	var chatMessage T

	// unmarshal the JSON into the struct
	err := json.Unmarshal([]byte(jsonText), &chatMessage)
	if err != nil {
		fmt.Println("Error:", err)
		return nil
	}

	return &chatMessage
}

func (m *ChatMessage) GetArguments() []string {
	return strings.Fields(m.Text)
}

/*

{"cmd":"chat","nick":"sky","uType":"mod","userid":2264580605166,"channel":"programming",
"text":" @gobot, has been seen online as: orangesun, gobot in last 15 minutes. ","level":999999,"flair":"⭐",
"mod":true,"trip":"595754","color":"BF40BF","time":1748116525374}
*/
