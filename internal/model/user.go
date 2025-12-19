package model

import (
	"encoding/json"
	"fmt"
)

type User struct {
	Channel string      `json:"channel"`
	Isme    bool        `json:"isme"`
	Name    string      `json:"nick"`
	Trip    string      `json:"trip"`
	UType   string      `json:"uType"`
	Hash    string      `json:"hash"`
	Level   int         `json:"level"`
	Color   string      `json:"color"`
	Flair   interface{} `json:"flair"`
	UserId  int64       `json:"userId"`
	IsBot   bool        `json:"isBot"`
}

func GetUsers(jsonData string) []*User {
	// Define the wrapper structure
	var result struct {
		Users []*User `json:"Users"`
	}

	// Unmarshal the JSON into the struct
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		fmt.Println("Error:", err)
		return nil
	}

	return result.Users
}

func GetUser(jsonData string) (*User, error) {
	var user User
	// Unmarshal the JSON into the struct
	err := json.Unmarshal([]byte(jsonData), &user)
	return &user, err
}

/*
online add: {"cmd":"onlineAdd","nick":"blahuser","trip":"","uType":"user","hash":"1EaG3s9EQge89i2","level":100,"userid":143778215917,"isBot":false,"color":"e6ed5e","flair":false,"channel":"programming","time":1748291833145}
Incoming message:  {"cmd":"info","channel":"programming","from":"blahuser","to":8710674673714,"text":"blahuser whispered: this is whisper text","type":"whisper","trip":"null","time":1748291847911}
info: {"cmd":"info","channel":"programming","from":"blahuser","to":8710674673714,"text":"blahuser whispered: this is whisper text","type":"whisper","trip":"null","time":1748291847911}
removed: {"cmd":"onlineRemove","nick":"blahuser","userid":143778215917,"time":1748291856499}
*/
