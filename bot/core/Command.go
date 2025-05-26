package core

import "zenbot/bot/model"

type Command interface {
	Execute()
	GetRole() *model.Role
}
