package command

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type commandDBZRepo struct{}

// dbzHelpSendErrorEngine is a post-implementation QA seam for the source
// command's exceptional enqueue path.
type dbzHelpSendErrorEngine struct {
	commandEngineStub
	err error
}

func (e *dbzHelpSendErrorEngine) SendChatMessage(string, string, bool) (string, error) {
	return "", e.err
}

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

type failingRegisterDBZRepo struct {
	commandDBZRepo
	name string
	err  error
}

func (r *failingRegisterDBZRepo) RegisterCharacter(_ context.Context, name string, _ func() int64) (int64, error) {
	r.name = name
	return 0, r.err
}

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
func TestRegisterUserUtilitiesRegistersDBZHelpWithoutDBZState(t *testing.T) {
	e := &commandEngineStub{}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"dbzhelp", "dbz", "dhelp"} {
		metadata, ok := (*e.GetEnabledCommands())[alias]
		if !ok {
			t.Fatalf("%s was not registered", alias)
		}
		command, ok := metadata.Command(&model.ChatMessage{Text: "!" + alias}).(*legacyAdapter)
		if !ok || command.def.Canonical != "dbzhelp" || command.def.Role != model.REGULAR {
			t.Fatalf("%s command=%#v", alias, command)
		}
	}
	for _, alias := range []string{"dbzstats", "dfight", "df"} {
		if _, ok := (*e.GetEnabledCommands())[alias]; ok {
			t.Fatalf("%s was registered without DBZ state", alias)
		}
	}
}

func TestDBZHelpIgnoresArgumentsAndPublishesExactSaturnPayload(t *testing.T) {
	e := &commandEngineStub{}
	d, ok := commandDefinitionFor("dhelp")
	if !ok {
		t.Fatal("dhelp definition is missing")
	}
	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dhelp ignored", IsWhisper: true}).Execute(context.Background())
	const want = `This is a DBZ universe text based game.
Main mechanics:
/train, - training your char in order to level up and gain point (stats)
/fight <nick>, - fight against a player
/claim - claim an item that just spawned
\u2009
/stats - displays character stats
/strength <int> - add a point into str
/agility <int> - add a point into agility
/vitality <int> - add a point into vitality
/energy <int> - add a point into energy
`
	if status != model.SUCCESSFUL || err != nil {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(e.chats) != 1 || e.chats[0] != "|"+want+"|false" {
		t.Fatalf("chats=%q", e.chats)
	}
}

// Post-implementation QA regression: its first run is expected to pass because
// the implementation evidence already established the no-bundle branch.
func TestDBZHelpReturnsFailedWhenPublicSendFails(t *testing.T) {
	wantErr := errors.New("output queue unavailable")
	e := &dbzHelpSendErrorEngine{err: wantErr}
	d, ok := commandDefinitionFor("dbzhelp")
	if !ok {
		t.Fatal("dbzhelp definition is missing")
	}

	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dbzhelp"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, wantErr) || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestRegisterUserUtilitiesRegistersDSpawnOnlyWhenDBZStateExists(t *testing.T) {
	withDBZ := &commandEngineStub{bundle: &service.Bundle{DBZ: &service.DBZService{}}}
	if err := RegisterUserUtilities(withDBZ); err != nil {
		t.Fatal(err)
	}
	metadata, ok := (*withDBZ.GetEnabledCommands())["dspawn"]
	if !ok {
		t.Fatal("dspawn was not registered with DBZ state")
	}
	command, ok := metadata.Command(&model.ChatMessage{Text: "!dspawn"}).(*legacyAdapter)
	if !ok || command.def.Canonical != "dspawn" || command.def.Role != model.REGULAR {
		t.Fatalf("dspawn command=%#v", command)
	}

	withoutDBZ := &commandEngineStub{}
	if err := RegisterUserUtilities(withoutDBZ); err != nil {
		t.Fatal(err)
	}
	if _, ok := (*withoutDBZ.GetEnabledCommands())["dspawn"]; ok {
		t.Fatal("dspawn was registered without DBZ state")
	}
}

func TestDSpawnMissingEnemyFailsWithSourceUsageAndDoesNotMutate(t *testing.T) {
	dbz := &service.DBZService{}
	e := &commandEngineStub{bundle: &service.Bundle{DBZ: dbz}}
	d, ok := commandDefinitionFor("dspawn")
	if !ok {
		t.Fatal("dspawn definition is missing")
	}

	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dspawn   ", IsWhisper: true}).Execute(context.Background())
	if status != model.FAILED || err != nil {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(e.chats) != 1 || e.chats[0] != "goku|Example: !dspawn enemy|true" {
		t.Fatalf("chats=%q", e.chats)
	}
	if enemies := dbz.Enemies(); len(enemies) != 0 {
		t.Fatalf("enemies=%q", enemies)
	}
}

func TestDSpawnUsesFirstTokenAppendsDuplicateAndPublishesPublicAcknowledgement(t *testing.T) {
	dbz := &service.DBZService{}
	e := &commandEngineStub{bundle: &service.Bundle{DBZ: dbz}}
	d, ok := commandDefinitionFor("dspawn")
	if !ok {
		t.Fatal("dspawn definition is missing")
	}
	for _, message := range []*model.ChatMessage{
		{Name: "goku", Text: "!dspawn   frieza ignored", IsWhisper: true},
		{Name: "goku", Text: "!dspawn frieza"},
	} {
		status, err := d.New(e, message).Execute(context.Background())
		if status != model.SUCCESSFUL || err != nil {
			t.Fatalf("message=%q status=%v err=%v", message.Text, status, err)
		}
	}
	if got, want := dbz.Enemies(), []string{"frieza", "frieza"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enemies=%q want=%q", got, want)
	}
	wantChat := "|spawned enemy: frieza|false"
	if len(e.chats) != 2 || e.chats[0] != wantChat || e.chats[1] != wantChat {
		t.Fatalf("chats=%q", e.chats)
	}
}

func TestDSpawnReturnsFailedAfterSpawnWhenPublicQueueFails(t *testing.T) {
	wantErr := errors.New("output queue unavailable")
	dbz := &service.DBZService{}
	e := &dbzHelpSendErrorEngine{
		commandEngineStub: commandEngineStub{bundle: &service.Bundle{DBZ: dbz}},
		err:               wantErr,
	}
	d, ok := commandDefinitionFor("dspawn")
	if !ok {
		t.Fatal("dspawn definition is missing")
	}

	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dspawn frieza"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, wantErr) {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if got, want := dbz.Enemies(), []string{"frieza"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enemies=%q want=%q", got, want)
	}
}

func TestDBZRegisterAcknowledgesSuccessWhenPersistenceFails(t *testing.T) {
	// Regression/provenance limitation: this first run is already green because
	// the command has always discarded the DBZ service error at its boundary.
	persistenceErr := errors.New("stats insert failed")
	repo := &failingRegisterDBZRepo{err: persistenceErr}
	e := &commandEngineStub{bundle: &service.Bundle{DBZ: &service.DBZService{Repo: repo}}}
	d, ok := commandDefinitionFor("dbzregister")
	if !ok {
		t.Fatal("dbzregister definition is missing")
	}

	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dbzregister ignored", IsWhisper: true}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || repo.name != "goku" {
		t.Fatalf("status=%v err=%v registered=%q", status, err, repo.name)
	}
	if len(e.chats) != 1 || e.chats[0] != "|Successfully registered character: goku|false" {
		t.Fatalf("chats=%q", e.chats)
	}
}

func TestDBZRegisterReturnsFailedAfterPersistenceWhenPublicSendFails(t *testing.T) {
	queueErr := errors.New("output queue unavailable")
	repo := &failingRegisterDBZRepo{err: errors.New("stats insert failed")}
	e := &dbzHelpSendErrorEngine{
		commandEngineStub: commandEngineStub{bundle: &service.Bundle{DBZ: &service.DBZService{Repo: repo}}},
		err:               queueErr,
	}
	d, ok := commandDefinitionFor("dbzregister")
	if !ok {
		t.Fatal("dbzregister definition is missing")
	}

	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dr"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, queueErr) || repo.name != "goku" {
		t.Fatalf("status=%v err=%v registered=%q", status, err, repo.name)
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

type freeStatsReadErrorCommandRepo struct{ commandDBZRepo }

func (freeStatsReadErrorCommandRepo) FreeStats(context.Context, string) (int, bool, error) {
	return 0, false, errors.New("read failed")
}

func TestDBZStrengthReadErrorPublishesSourceEquivalentPublicNoFreeSuccess(t *testing.T) {
	e := &commandEngineStub{bundle: &service.Bundle{DBZ: &service.DBZService{Repo: freeStatsReadErrorCommandRepo{}}}}
	d, _ := commandDefinitionFor("dstr")
	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dstr 1", IsWhisper: true}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || len(e.chats) != 1 || e.chats[0] != "|You don't have free stats. Level up!|false" {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

type writeErrorStrengthCommandRepo struct {
	commandDBZRepo
	strengthAdds int
}

func (r *writeErrorStrengthCommandRepo) AddStrength(context.Context, string, int) error {
	r.strengthAdds++
	return errors.New("strength write failed")
}

// Post-implementation QA regression: the source command deliberately ignores
// the storage write result and retains the public acknowledgement.
func TestDBZStrengthAcknowledgesPublicAmountWhenWriteFails(t *testing.T) {
	repo := &writeErrorStrengthCommandRepo{}
	e := &commandEngineStub{bundle: &service.Bundle{DBZ: &service.DBZService{Repo: repo}}}
	d, _ := commandDefinitionFor("dstr")
	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dstr 1", IsWhisper: true}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || repo.strengthAdds != 1 || len(e.chats) != 1 || e.chats[0] != "|1|false" {
		t.Fatalf("status=%v err=%v adds=%d chats=%v", status, err, repo.strengthAdds, e.chats)
	}
}

type failingLevelUpDBZRepo struct {
	commandDBZRepo
	calls int
	err   error
}

func (r *failingLevelUpDBZRepo) LevelUp(context.Context, string) error {
	r.calls++
	return r.err
}

func TestDBZFightAcknowledgesAfterLevelUpErrorAndConsumesMatchingEnemy(t *testing.T) {
	// Regression/provenance: source-shaped command behavior already discards the
	// level-up error; this protects that boundary.
	repo := &failingLevelUpDBZRepo{err: errors.New("level update failed")}
	dbz := &service.DBZService{Repo: repo}
	dbz.SpawnEnemy("frieza")
	e := &commandEngineStub{bundle: &service.Bundle{DBZ: dbz}}
	d, ok := commandDefinitionFor("dfight")
	if !ok {
		t.Fatal("dfight definition is missing")
	}

	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dfight frieza", IsWhisper: true}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || repo.calls != 1 {
		t.Fatalf("status=%v err=%v level-up calls=%d", status, err, repo.calls)
	}
	if enemies := dbz.Enemies(); len(enemies) != 0 {
		t.Fatalf("enemies=%q, want matching enemy consumed", enemies)
	}
	if len(e.chats) != 1 || e.chats[0] != "|Gz. Enemy has been slain. Your leveled up! Granted 5 free stats!|false" {
		t.Fatalf("chats=%q", e.chats)
	}
}

func TestDBZFightReturnsFailedAfterEffectsWhenPublicSendFails(t *testing.T) {
	queueErr := errors.New("output queue unavailable")
	repo := &failingLevelUpDBZRepo{}
	dbz := &service.DBZService{Repo: repo}
	dbz.SpawnEnemy("frieza")
	e := &dbzHelpSendErrorEngine{
		commandEngineStub: commandEngineStub{bundle: &service.Bundle{DBZ: dbz}},
		err:               queueErr,
	}
	d, ok := commandDefinitionFor("dfight")
	if !ok {
		t.Fatal("dfight definition is missing")
	}

	status, err := d.New(e, &model.ChatMessage{Name: "goku", Text: "!dfight frieza"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, queueErr) || repo.calls != 1 {
		t.Fatalf("status=%v err=%v level-up calls=%d", status, err, repo.calls)
	}
	if enemies := dbz.Enemies(); len(enemies) != 0 {
		t.Fatalf("enemies=%q, want matching enemy consumed before send", enemies)
	}
}
