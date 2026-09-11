package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type mailCommand struct{ commandBase }

func (c *mailCommand) Execute(ctx context.Context) (model.Status, error) {
	if e := ctx.Err(); e != nil {
		return model.FAILED, e
	}
	receiver, body := splitCommandToken(commandBody(c.message))
	body = strings.TrimSpace(body)
	if receiver == "" {
		if err := replyContext(ctx, &c.commandBase, "Example: -mail merc message"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Mail == nil {
		return model.FAILED, fmt.Errorf("mail service unavailable")
	}
	receivers, e := b.Mail.QueueResolved(ctx, body, c.message.Name+"#"+c.message.Trip, receiver, true)
	if e != nil {
		if errors.Is(e, service.ErrMailReceiverBlank) {
			if err := replyContext(ctx, &c.commandBase, "Receiver cannot be blank."); err != nil {
				return model.FAILED, errors.Join(e, err)
			}
		} else if errors.Is(e, service.ErrMailRecipientUnregistered) {
			users, listErr := b.Mail.SaturnRegisteredUsers(ctx)
			if listErr != nil {
				return model.FAILED, errors.Join(e, listErr)
			}
			if err := replyContext(ctx, &c.commandBase, "User you specified is not registered. Please use a name from provided list to send a message to respective trip. \\\\n"+formatSaturnRegisteredUsers(users)); err != nil {
				return model.FAILED, errors.Join(e, err)
			}
		} else {
			return model.FAILED, e
		}
		return model.FAILED, nil
	}
	if err := replyContext(ctx, &c.commandBase, "trips: "+receivers+" will receive your message as soon they chat"); err != nil {
		return model.FAILED, err
	}
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
	text := commandBody(c.message)
	if text == "" {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"note Jedi am I?!"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	if strings.TrimSpace(c.message.Trip) == "" {
		if err := replyContext(ctx, &c.commandBase, "Set your trip before saving a note."); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Notes == nil {
		return model.FAILED, fmt.Errorf("note service unavailable")
	}
	if e := b.Notes.Save(ctx, c.message.Trip, text); e != nil {
		return model.FAILED, e
	}
	if err := replyContext(ctx, &c.commandBase, "note successfully saved!"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type notesCommand struct{ commandBase }

func (c *notesCommand) Execute(ctx context.Context) (model.Status, error) {
	if e := ctx.Err(); e != nil {
		return model.FAILED, e
	}
	if c.message.Trip == "" {
		if err := replyContext(ctx, &c.commandBase, "\\n Set your trip first. Example: !notes"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	b := bundle(c.engine)
	if b == nil || b.Notes == nil {
		return model.FAILED, fmt.Errorf("note service unavailable")
	}
	a := args(c.message)
	if len(a) > 0 && (a[0] == "purge" || a[0] == "clear") {
		if e := b.Notes.Clear(ctx, c.message.Trip); e != nil {
			return model.FAILED, e
		}
		if err := replyContext(ctx, &c.commandBase, "'s notes has been deleted"); err != nil {
			return model.FAILED, err
		}
		return model.SUCCESSFUL, nil
	}
	if len(a) > 0 {
		return model.FAILED, nil
	}
	ns, e := b.Notes.List(ctx, c.message.Trip)
	if e != nil {
		return model.FAILED, e
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, "'s notes: \\n ```Text \\n"+fmt.Sprint(ns)+"\\n```", true); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}
