package info

import (
	"context"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type renameAfkEngine struct {
	common.Engine
	name          string
	before, after string
}

func (e *renameAfkEngine) GetName() string     { return e.name }
func (e *renameAfkEngine) SetName(name string) { e.name = name }
func (e *renameAfkEngine) RenameAfkUser(before, after string) {
	e.before, e.after = before, after
}

func TestRenameAfkUsersUpdatesBotAndAfkIdentity(t *testing.T) {
	engine := &renameAfkEngine{name: "old-bot"}
	next, err := (RenameAfkUsers{}).Handle(context.Background(), &Context{Engine: engine, Message: &model.InfoMessage{Text: "old-bot is now new-bot"}})
	if err != nil || !next || engine.name != "new-bot" || engine.before != "old-bot" || engine.after != "new-bot" {
		t.Fatalf("next=%v err=%v engine=%+v", next, err, engine)
	}
}

type whisperAuditEngine struct {
	common.Engine
	record        model.MessageRecord
	fallbackCalls int
}

func (*whisperAuditEngine) GetChannel() string { return "programming" }
func (e *whisperAuditEngine) LogMessage(string, string, string, string, string) (int64, error) {
	e.fallbackCalls++
	return 0, nil
}
func (e *whisperAuditEngine) LogMessageRecord(_ context.Context, record model.MessageRecord) (int64, error) {
	e.record = record
	return 1, nil
}

func TestAuditWhisperCommandPersistsWhisperVisibility(t *testing.T) {
	engine := &whisperAuditEngine{}
	chat := &model.ChatMessage{Name: "alice", Trip: "trip", Hash: "hash", Text: "secret"}
	next, err := (AuditWhisperCommand{}).Handle(context.Background(), &Context{Engine: engine, ChatMessage: chat})
	if err != nil || !next {
		t.Fatalf("next=%v err=%v", next, err)
	}
	if engine.fallbackCalls != 0 || engine.record.Visibility != "WHISPER" || engine.record.Message != "secret" || engine.record.Channel != "programming" || engine.record.CreatedOnMillis <= 0 {
		t.Fatalf("record=%+v", engine.record)
	}
}
