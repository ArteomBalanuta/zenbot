package factory

import (
	"context"
	"database/sql"
	"testing"

	"zenbot/internal/config"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type groupBEngineRepo struct{}

func (groupBEngineRepo) LogMessage(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (groupBEngineRepo) LogPresence(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (groupBEngineRepo) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	return 0, nil
}
func (groupBEngineRepo) Close() error   { return nil }
func (groupBEngineRepo) SQLDB() *sql.DB { return nil }
func (groupBEngineRepo) DeleteIdentity(context.Context, string, string) (repository.DeleteResult, error) {
	return repository.DeleteResult{}, nil
}
func (groupBEngineRepo) SaturnRegisteredUsers(context.Context) ([]repository.SaturnRegisteredUser, error) {
	return nil, nil
}
func (groupBEngineRepo) SaturnLastMessages(context.Context, *string, string, int) ([]repository.SaturnLastMessage, error) {
	return nil, nil
}

type legacyOnlyEngineRepo struct{}

func (legacyOnlyEngineRepo) LogMessage(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (legacyOnlyEngineRepo) LogPresence(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (legacyOnlyEngineRepo) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	return 0, nil
}
func (legacyOnlyEngineRepo) Close() error { return nil }

func TestNewEngineInjectsGroupBRepositoryIntoTypedServiceOwners(t *testing.T) {
	cfg := &config.Config{WebsocketUrl: "ws://127.0.0.1:1", Channel: "test"}
	repo := groupBEngineRepo{}
	e, err := NewEngineWithOptions(model.MASTER, cfg, repo, EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if e.Services == nil || e.Services.Users.GroupB != repo || e.Services.Mail.GroupB != repo {
		t.Fatalf("services=%+v", e.Services)
	}
}

func TestNewEngineLegacyOnlyRepositoryLeavesGroupBNil(t *testing.T) {
	cfg := &config.Config{WebsocketUrl: "ws://127.0.0.1:1", Channel: "test"}
	e, err := NewEngineWithOptions(model.MASTER, cfg, legacyOnlyEngineRepo{}, EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if e.Services != nil {
		t.Fatalf("expected no database-backed services, got=%+v", e.Services)
	}
}
