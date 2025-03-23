package main

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

func GetUsers(jsonData string) *[]User {
	// Define the wrapper structure
	var result struct {
		Users []User `json:"Users"`
	}

	// Unmarshal the JSON into the struct
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		fmt.Println("Error:", err)
		return nil
	}

	return &result.Users
}
