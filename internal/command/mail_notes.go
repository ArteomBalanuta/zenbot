package command

import (
	"context"
	"fmt"
	"strings"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type mailCommand struct{ commandBase }

func (c *mailCommand) Execute(ctx context.Context) (model.Status, error) {
	if e := ctx.Err(); e != nil {
		return model.FAILED, e
	}
	a := args(c.message)
	if len(a) == 0 {
		reply(&c.commandBase, "Example: -mail merc message")
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Mail == nil {
		return model.FAILED, fmt.Errorf("mail service unavailable")
	}
	receivers, e := b.Mail.QueueResolved(strings.Join(a[1:], " "), c.message.Name+"#"+c.message.Trip, a[0], c.message.IsWhisper)
	if e != nil {
		if e.Error() == "receiver cannot be blank" {
			reply(&c.commandBase, "Receiver cannot be blank.")
		} else if e.Error() == "user not registered" {
			users, listErr := b.Mail.SaturnRegisteredUsers(ctx)
			if listErr != nil {
				return model.FAILED, listErr
			}
			reply(&c.commandBase, "User you specified is not registered. Please use a name from provided list to send a message to respective trip. \\\\n"+formatSaturnRegisteredUsers(users))
		} else {
			return model.FAILED, e
		}
		return model.SUCCESSFUL, nil
	}
	reply(&c.commandBase, "trips: "+receivers+" will receive your message as soon they chat")
	return model.SUCCESSFUL, nil
}

func formatSaturnRegisteredUsers(users []repository.SaturnRegisteredUser) string {
	var directory strings.Builder
	for _, user := range users {
		directory.WriteString(user.Name + " " + user.Trip + "\\n")
	}
	return directory.String()
}

type noteCommand struct{ commandBase }

func (c *noteCommand) Execute(ctx context.Context) (model.Status, error) {
	if e := ctx.Err(); e != nil {
		return model.FAILED, e
	}
	a := args(c.message)
	if len(a) == 0 {
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"note Jedi am I?!")
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Notes == nil {
		return model.FAILED, fmt.Errorf("note service unavailable")
	}
	var e error
	if c.message.Trip != "" {
		e = b.Notes.Save(c.message.Trip, strings.Join(a, " "))
	}
	if e != nil {
		return model.FAILED, e
	}
	reply(&c.commandBase, "note successfully saved!")
	return model.SUCCESSFUL, nil
}

type notesCommand struct{ commandBase }

func (c *notesCommand) Execute(ctx context.Context) (model.Status, error) {
	if e := ctx.Err(); e != nil {
		return model.FAILED, e
	}
	if c.message.Trip == "" {
		reply(&c.commandBase, "\\n Set your trip first. Example: !notes")
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Notes == nil {
		return model.FAILED, fmt.Errorf("note service unavailable")
	}
	a := args(c.message)
	if len(a) > 0 && (a[0] == "purge" || a[0] == "clear") {
		if e := b.Notes.Clear(c.message.Trip); e != nil {
			return model.FAILED, e
		}
		reply(&c.commandBase, "'s notes has been deleted")
		return model.SUCCESSFUL, nil
	}
	if len(a) > 0 {
		return model.FAILED, nil
	}
	ns, e := b.Notes.List(c.message.Trip)
	if e != nil {
		return model.FAILED, e
	}
	reply(&c.commandBase, "'s notes: \\n ```Text \\n"+fmt.Sprint(ns)+"\\n```")
	return model.SUCCESSFUL, nil
}
