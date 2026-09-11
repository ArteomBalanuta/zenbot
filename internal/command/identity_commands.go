package command

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/util"
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
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"reg merc g0KY09"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	s := userService(c.engine)
	if s == nil || s.Identity == nil {
		return model.FAILED, fmt.Errorf("user service unavailable")
	}
	name, err := util.NormalizeNickTarget(&a[0])
	if err != nil {
		return model.FAILED, err
	}
	trip := strings.TrimSpace(a[1])
	n, err := s.IsNameRegistered(ctx, name)
	if err != nil {
		return model.FAILED, err
	}
	t, err := s.IsTripRegistered(ctx, trip)
	if err != nil {
		return model.FAILED, err
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	var acknowledgment string
	switch {
	case !n && !t:
		err = s.Register(ctx, name, trip, model.REGULAR)
		acknowledgment = "User has been registered successfully, now you can msg him by name: " + name
	case !n:
		err = s.RegisterNameByTrip(ctx, name, trip)
		acknowledgment = fmt.Sprintf("New name: %s, assigned to trip: %s", name, trip)
	case !t:
		err = s.RegisterTripByName(ctx, name, trip)
		acknowledgment = fmt.Sprintf("New trip: %s, assigned to user named: %s", trip, name)
	default:
		if err := replyContext(ctx, &c.commandBase, fmt.Sprintf("Name %s and trip %s are already registered.", name, trip)); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	if err != nil {
		if sendErr := replyContext(ctx, &c.commandBase, "Something went wrong"); sendErr != nil {
			return model.FAILED, errors.Join(err, sendErr)
		}
		return model.FAILED, err
	}
	// Registration is committed here, before attempting its acknowledgment.
	if err := replyContext(ctx, &c.commandBase, acknowledgment); err != nil {
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
		if err := replyContext(ctx, &c.commandBase, " example: "+c.engine.GetPrefix()+"auth cmdTV+"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Security == nil {
		return model.FAILED, fmt.Errorf("security service unavailable")
	}
	trip := strings.TrimSpace(a[0])
	if err := b.Security.AuthorizeTripContext(ctx, trip); err != nil {
		return model.FAILED, err
	}
	if err := replyContext(ctx, &c.commandBase, " authorized trip: "+trip); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type accessCommand struct{ commandBase }

func (c *accessCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) != 2 || strings.TrimSpace(c.message.Trip) == "" {
		if err := replyContext(ctx, &c.commandBase, "\n Set your trip first. Example: "+c.engine.GetPrefix()+"grant 8Wotmg ADMIN"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	roleName := a[1]
	role, ok := parseRole(roleName)
	if !ok {
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Security == nil || b.Security.Authorization == nil {
		return model.FAILED, fmt.Errorf("security service unavailable")
	}
	target := strings.TrimSpace(a[0])
	trips := strings.Split(strings.TrimRight(target, ","), ",")
	targets := make([]string, 0, len(trips))
	seen := make(map[string]bool, len(trips))
	for _, trip := range trips {
		trip = strings.TrimSpace(trip)
		if trip == "" {
			return model.FAILED, fmt.Errorf("each target must be a nonempty trip")
		}
		if !seen[trip] {
			seen[trip] = true
			targets = append(targets, trip)
		}
	}
	if err := b.Security.Authorization.GrantTrips(ctx, targets, role); err != nil {
		return model.FAILED, err
	}
	// The entire role batch is committed here, before its acknowledgment.
	common.RecordCommittedMutation(ctx)
	acknowledgment := "\n Granted new Role: " + roleName + " to trip: " + target
	if strings.Contains(target, ",") {
		acknowledgment = fmt.Sprintf("\n Granted new Roles: %s to trips: %v", roleName, targets)
	}
	if err := replyContext(ctx, &c.commandBase, acknowledgment); err != nil {
		return model.FAILED, err
	}
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
		if _, err := c.send(ctx, "Example: "+c.engine.GetPrefix()+"lastmessages g0KY09 3"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(a[1]))
	if err != nil || n <= 0 {
		if _, sendErr := c.send(ctx, "Example: "+c.engine.GetPrefix()+"lastmessages g0KY09 3"); sendErr != nil {
			return model.FAILED, sendErr
		}
		return model.FAILED, nil
	}
	s := userService(c.engine)
	if s == nil || (!configuredDependency(s.GroupB) && !configuredDependency(s.Identity)) {
		return model.FAILED, fmt.Errorf("user service unavailable")
	}
	if n > 30 {
		if _, err := c.send(ctx, "Retrieving at max 30 messages! "); err != nil {
			return model.FAILED, err
		}
		n = 30
	}
	var ms []repository.SaturnLastMessage
	if configuredDependency(s.GroupB) {
		ms, err = s.SaturnLastMessages(ctx, nil, strings.TrimSpace(a[0]), n)
	} else {
		var legacy []model.Message
		legacy, err = s.LastMessages(ctx, "", strings.TrimSpace(a[0]), n)
		for _, m := range legacy {
			ms = append(ms, repository.SaturnLastMessage{Name: m.Name, Trip: m.Trip, Message: m.Message, CreatedOn: m.CreatedOn})
		}
	}
	if err != nil {
		return model.FAILED, err
	}
	whisper := c.message.Whisper || c.message.IsWhisper || c.message.Type == "whisper"
	if len(ms) == 0 {
		if err := observeAndReply(ctx, &c.commandBase, "No messages found.", whisper); err != nil {
			return model.FAILED, err
		}
		return model.SUCCESSFUL, nil
	}
	var b strings.Builder
	for _, m := range ms {
		msg := truncateUTF8Bytes(m.Message, 200)
		b.WriteString("\n")
		b.WriteString(m.Name + "#" + m.Trip + ": " + msg)
		b.WriteString("\n")
	}
	if err := observeAndReply(ctx, &c.commandBase, b.String(), whisper); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func (c *messagesCommand) send(ctx context.Context, text string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return c.engine.SendChatMessage(c.message.Name, text, c.message.Whisper || c.message.IsWhisper || c.message.Type == "whisper")
}

func truncateUTF8Bytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	boundary := limit
	for boundary > 0 && !utf8.RuneStart(value[boundary]) {
		boundary--
	}
	return value[:boundary] + "..."
}
