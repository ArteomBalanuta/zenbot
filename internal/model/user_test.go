package model

import (
	"reflect"
	"testing"
)

func Test_GetUser(t *testing.T) {
	actual, err := GetUser(`{"cmd":"onlineAdd","nick":"blahuser","trip":"","uType":"user","hash":"1EaG3s9EQge89i2","level":100,"userid":143778215917,"isBot":false,"color":"e6ed5e","flair":false,"channel":"programming","time":1748291833145}`)

	expected := User{
		Channel: "programming",
		Isme:    false,
		Name:    "blahuser",
		Trip:    "",
		UType:   "user",
		Hash:    "1EaG3s9EQge89i2",
		Level:   100,
		Color:   "e6ed5e",
		Flair:   false,
		UserId:  143778215917,
		IsBot:   false,
	}

	if err != nil {
		t.Errorf("GetUser() = %+v; want %+v", actual, expected)
	}

	if !reflect.DeepEqual(*actual, expected) {
		t.Errorf("GetUser() = %+v; want %+v", actual, expected)
	}
}
