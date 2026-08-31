package command

import (
	"context"
	"strings"

	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type usersCommand struct{ commandBase }

func (c *usersCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	b := bundle(c.engine)
	if b == nil || b.Users == nil {
		return model.SUCCESSFUL, nil
	}
	var users []repository.RegisteredUser
	if b.Users.GroupB != nil {
		groupUsers, err := b.Users.SaturnRegisteredUsers(ctx)
		if err != nil {
			return model.FAILED, err
		}
		users = make([]repository.RegisteredUser, 0, len(groupUsers))
		for _, user := range groupUsers {
			users = append(users, repository.RegisteredUser{Name: user.Name, Trip: user.Trip})
		}
	} else if b.Users.Queries != nil {
		var err error
		users, err = b.Users.RegisteredUsers(ctx)
		if err != nil {
			return model.FAILED, err
		}
	} else {
		return model.SUCCESSFUL, nil
	}
	reply(&c.commandBase, "Users: \\n"+formatRegisteredUsers(users))
	return model.SUCCESSFUL, nil
}

type nicksCommand struct{ commandBase }

func (c *nicksCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || strings.TrimSpace(a[0]) == "" {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"t2n QLnV66")
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Users == nil || b.Users.Queries == nil {
		return model.SUCCESSFUL, nil
	}
	nicks, err := b.Users.NicksByTrip(ctx, strings.TrimSpace(a[0]))
	if err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, strings.Join(nicks, ","))
	return model.SUCCESSFUL, nil
}

func formatRegisteredUsers(users []repository.RegisteredUser) string {
	widthTrip, widthName := len("TRIP"), len("NAME")
	for _, user := range users {
		if len(user.Trip) > widthTrip {
			widthTrip = len(user.Trip)
		}
		if len(user.Name) > widthName {
			widthName = len(user.Name)
		}
	}
	if widthTrip%2 != 0 {
		widthTrip++
	}
	if widthName%2 != 0 {
		widthName++
	}
	line := func() string {
		return "+" + strings.Repeat("-", widthTrip+4) + "+" + strings.Repeat("-", widthName+4) + "+"
	}
	cell := func(value string, width int) string {
		padding := 2 + (width-len(value))/2
		extra := ""
		if len(value)%2 != 0 {
			extra = " "
		}
		return strings.Repeat(" ", padding) + value + extra + strings.Repeat(" ", padding)
	}
	var b strings.Builder
	b.WriteString("\\n\\n")
	b.WriteString(line())
	b.WriteString("\\n")
	b.WriteString("|" + cell("TRIP", widthTrip) + "|" + cell("NAME", widthName) + "|")
	b.WriteString("\\n")
	b.WriteString(line())
	for _, user := range users {
		b.WriteString("\\n")
		b.WriteString("|" + cell(user.Trip, widthTrip) + "|" + cell(user.Name, widthName) + "|")
	}
	b.WriteString("\\n")
	b.WriteString(line())
	b.WriteString("\\n\\n")
	return b.String()
}
