package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

func userService(e common.Engine) *service.UserService {
	b := bundle(e)
	if b == nil {
		return nil
	}
	return b.Users
}

type registerCommand struct{ commandBase }

func (c *registerCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) < 2 {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"reg merc g0KY09")
		return model.FAILED, nil
	}
	s := userService(c.engine)
	if s == nil {
		return model.FAILED, fmt.Errorf("user service unavailable")
	}
	name, trip := strings.TrimSpace(a[0]), strings.TrimSpace(a[1])
	n, err := s.IsNameRegistered(name)
	if err != nil {
		return model.FAILED, err
	}
	t, err := s.IsTripRegistered(trip)
	if err != nil {
		return model.FAILED, err
	}
	switch {
	case !n && !t:
		err = s.Register(name, trip, model.REGULAR)
		if err == nil {
			reply(&c.commandBase, "User has been registered successfully, now you can msg him by name: "+name)
		}
	case !n:
		err = s.RegisterNameByTrip(name, trip)
		if err == nil {
			reply(&c.commandBase, fmt.Sprintf("New name: %s, assigned to trip: %s", name, trip))
		}
	case !t:
		err = s.RegisterTripByName(name, trip)
		if err == nil {
			reply(&c.commandBase, fmt.Sprintf("New trip: %s, assigned to user named: %s", trip, name))
		}
	default:
		reply(&c.commandBase, fmt.Sprintf("Name %s and trip %s are already registered.", name, trip))
		return model.FAILED, nil
	}
	if err != nil {
		reply(&c.commandBase, "Something went wrong")
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type authorizeCommand struct{ commandBase }

func (c *authorizeCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 {
		reply(&c.commandBase, " example: "+c.engine.GetPrefix()+"auth cmdTV+")
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Security == nil {
		return model.FAILED, fmt.Errorf("security service unavailable")
	}
	trip := strings.TrimSpace(a[0])
	if err := b.Security.AuthorizeTrip(trip); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, " authorized trip: "+trip)
	return model.SUCCESSFUL, nil
}

type accessCommand struct{ commandBase }

func (c *accessCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) != 2 || strings.TrimSpace(c.message.Trip) == "" {
		reply(&c.commandBase, "\\n Set your trip first. Example: "+c.engine.GetPrefix()+"grant 8Wotmg ADMIN")
		return model.FAILED, nil
	}
	roleName := a[1]
	role, ok := parseRole(roleName)
	if !ok {
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Security == nil {
		return model.FAILED, fmt.Errorf("security service unavailable")
	}
	target := strings.TrimSpace(a[0])
	if strings.Contains(target, ",") {
		trips := strings.Split(target, ",")
		for len(trips) > 0 && trips[len(trips)-1] == "" {
			trips = trips[:len(trips)-1]
		}
		for _, trip := range trips {
			b.Security.Authorization.GrantTrip(ctx, trip, model.USER)
		}
		reply(&c.commandBase, fmt.Sprintf("\\n Granted new Roles: %s to trips: %v", roleName, trips))
		return model.SUCCESSFUL, nil
	}
	if err := b.Security.Authorization.GrantTrip(ctx, target, role); err != nil {
		return model.FAILED, err
	}
	reply(&c.commandBase, "\\n Granted new Role: "+roleName+" to trip: "+target)
	return model.SUCCESSFUL, nil
}
func parseRole(s string) (model.Role, bool) {
	switch s {
	case "ADMIN":
		return model.ADMIN, true
	case "MODERATOR":
		return model.MODERATOR, true
	case "TRUSTED":
		return model.TRUSTED, true
	case "USER":
		return model.USER, true
	case "REGULAR":
		return model.REGULAR, true
	case "PEST":
		return model.PEST, true
	}
	return model.REGULAR, false
}

type messagesCommand struct{ commandBase }

func (c *messagesCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) < 2 {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"lastmessages g0KY09 3")
		return model.FAILED, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(a[1]))
	if err != nil {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"lastmessages g0KY09 3")
		return model.FAILED, nil
	}
	if n > 30 {
		reply(&c.commandBase, "Retrieving at max 30 messages! ")
		n = 30
	}
	s := userService(c.engine)
	if s == nil {
		return model.FAILED, fmt.Errorf("user service unavailable")
	}
	var ms []repository.SaturnLastMessage
	if s.GroupB != nil {
		ms, err = s.SaturnLastMessages(ctx, nil, strings.TrimSpace(a[0]), n)
	} else {
		var legacy []model.Message
		legacy, err = s.LastMessages("", strings.TrimSpace(a[0]), n)
		for _, m := range legacy {
			ms = append(ms, repository.SaturnLastMessage{Name: m.Name, Trip: m.Trip, Message: m.Message, CreatedOn: m.CreatedOn})
		}
	}
	if err != nil {
		return model.FAILED, err
	}
	var b strings.Builder
	for _, m := range ms {
		msg := m.Message
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		b.WriteString("\n")
		b.WriteString(m.Name + "#" + m.Trip + ": " + msg)
		b.WriteString("\n")
	}
	reply(&c.commandBase, escapeJava(b.String()))
	return model.SUCCESSFUL, nil
}
func escapeJava(s string) string { return strconv.Quote(s)[1 : len(strconv.Quote(s))-1] }
