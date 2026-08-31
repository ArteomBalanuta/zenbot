package command

import (
	"context"
	"testing"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type commandDBZRepo struct{}

func (commandDBZRepo) RegisterCharacter(context.Context, string, func() int64) (int64, error) {
	return 1, nil
}
func (commandDBZRepo) LevelUp(context.Context, string) error          { return nil }
func (commandDBZRepo) AddStrength(context.Context, string, int) error { return nil }
func (commandDBZRepo) AddAgility(context.Context, string, int) error  { return nil }
func (commandDBZRepo) AddVitality(context.Context, string, int) error { return nil }
func (commandDBZRepo) AddEnergy(context.Context, string, int) error   { return nil }
func (commandDBZRepo) Stats(context.Context, string) (repository.DBZStats, bool, error) {
	return repository.DBZStats{Name: "a", Level: 1}, true, nil
}
func (commandDBZRepo) FreeStats(context.Context, string) (int, bool, error) { return 1, true, nil }
func TestDBZAliasesAreRegularAndConcrete(t *testing.T) {
	for _, a := range []string{"dbzregister", "dreg", "dr", "dbzstats", "dstats", "dstat", "ds", "dbzstr", "dstr", "daddstr", "dfight", "df", "dbzhelp", "dbz", "dhelp", "dspawn"} {
		d, ok := commandDefinitionFor(a)
		if !ok || d.Role != model.REGULAR {
			t.Fatalf("%s definition=%v ok=%v", a, d.Role, ok)
		}
		e := &commandEngineStub{bundle: &service.Bundle{DBZ: &service.DBZService{Repo: commandDBZRepo{}}}, users: map[string]*model.User{"a": {Name: "a"}}}
		_ = d.New(e, &model.ChatMessage{Name: "a", Text: "!" + a})
	}
}
func TestDBZMalformedStrengthUsesSourceUsage(t *testing.T) {
	e := &commandEngineStub{bundle: &service.Bundle{DBZ: &service.DBZService{Repo: commandDBZRepo{}}}}
	d, _ := commandDefinitionFor("dstr")
	st, err := d.New(e, &model.ChatMessage{Name: "a", Text: "!dstr nope"}).Execute(context.Background())
	if st != model.FAILED || err != nil || len(e.chats) != 1 || e.chats[0] != "a|Example: !daddstr amount|false" {
		t.Fatalf("status=%v err=%v chats=%v", st, err, e.chats)
	}
}
