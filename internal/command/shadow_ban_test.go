package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type shadowBanRepositoryStub struct {
	records        []repository.ShadowBanRecord
	persisted      []repository.ShadowBanRecord
	removedTarget  string
	removeAlls     int
	removeCount    int64
	removeAllCount int64
	listCalls      int
	err            error
	afterPersist   func()
	afterRemove    func()
}

func (s *shadowBanRepositoryStub) PersistShadowBanRecord(_ context.Context, record repository.ShadowBanRecord) error {
	if s.err != nil {
		return s.err
	}
	s.persisted = append(s.persisted, record)
	if s.afterPersist != nil {
		s.afterPersist()
	}
	return nil
}
func (s *shadowBanRepositoryStub) ListShadowBans(context.Context) ([]repository.ShadowBanRecord, error) {
	s.listCalls++
	if s.err != nil {
		return nil, s.err
	}
	return append([]repository.ShadowBanRecord(nil), s.records...), nil
}

func (s *shadowBanRepositoryStub) HasShadowBanMatch(ctx context.Context, trip, name, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s.err != nil {
		return false, s.err
	}
	for _, record := range s.records {
		if (strings.TrimSpace(trip) != "" && trip == record.Trip) || (strings.TrimSpace(name) != "" && name == record.Name) || (strings.TrimSpace(hash) != "" && hash == record.Hash) {
			return true, nil
		}
	}
	return false, nil
}
func (s *shadowBanRepositoryStub) RemoveShadowBanBySourceTarget(_ context.Context, target string) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.removedTarget = target
	if s.afterRemove != nil {
		s.afterRemove()
	}
	return s.removeCount, nil
}
func (s *shadowBanRepositoryStub) RemoveAllShadowBans(context.Context) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.removeAlls++
	if s.afterRemove != nil {
		s.afterRemove()
	}
	return s.removeAllCount, nil
}

type shadowBanCommandEngine struct {
	*commandEngineStub
	kicked      []string
	kickErr     error
	kickErrors  []error
	afterKick   func()
	afterLookup func()
	replyErr    error
	activeBy    map[string]*model.User
}

func (e *shadowBanCommandEngine) KickNick(_ context.Context, target common.NickTarget) error {
	e.kicked = append(e.kicked, string(target))
	if e.afterKick != nil {
		e.afterKick()
		e.afterKick = nil
	}
	if index := len(e.kicked) - 1; index < len(e.kickErrors) {
		return e.kickErrors[index]
	}
	return e.kickErr
}

func (e *shadowBanCommandEngine) SendChatMessage(author, text string, whisper bool) (string, error) {
	e.chats = append(e.chats, author+"|"+text+"|"+boolString(whisper))
	return text, e.replyErr
}

func (e *shadowBanCommandEngine) GetActiveUserByName(name string) *model.User {
	if e.afterLookup != nil {
		e.afterLookup()
		e.afterLookup = nil
	}
	return e.activeBy[name]
}

func newShadowBanEngine(repo *shadowBanRepositoryStub, users map[string]*model.User) *shadowBanCommandEngine {
	return &shadowBanCommandEngine{
		commandEngineStub: &commandEngineStub{
			users:  users,
			bundle: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: repo}},
		},
		activeBy: users,
	}
}

type shadowBanStorageOnlyEngine struct {
	common.Engine
	bundle   *service.Bundle
	commands map[string]common.CommandMetadata
}

func (e *shadowBanStorageOnlyEngine) ServiceBundle() *service.Bundle { return e.bundle }
func (e *shadowBanStorageOnlyEngine) RegisterCommand(c common.Command) {
	if e.commands == nil {
		e.commands = map[string]common.CommandMetadata{}
	}
	for _, alias := range c.GetAliases() {
		e.commands[alias] = common.CommandMetadata{}
	}
}

func TestRegisterUserUtilitiesDoesNotExposeShadowBanCommandsWithoutTypedKick(t *testing.T) {
	repo := &shadowBanRepositoryStub{}
	engine := &shadowBanStorageOnlyEngine{bundle: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: repo}}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"shadowban", "sban"} {
		if _, registered := engine.commands[alias]; registered {
			t.Errorf("shadow-ban alias %q registered without typed kick capability", alias)
		}
	}
	for _, alias := range []string{"shadowbanlist", "banlist", "bannedusers", "unshadowban", "shadowmercy", "unblock"} {
		if _, registered := engine.commands[alias]; !registered {
			t.Errorf("storage-only shadow-ban alias %q was not registered", alias)
		}
	}
}

func TestShadowBanListRendersDecodedRecordsAndNoBans(t *testing.T) {
	repo := &shadowBanRepositoryStub{records: []repository.ShadowBanRecord{{Hash: "hash", Trip: "", Name: "nick"}}}
	engine := newShadowBanEngine(repo, nil)
	definition, _ := commandDefinitionFor("banlist")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!banlist", IsWhisper: true}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(engine.chats, []string{"mod|Banned hashes, trips, names: \\nhash - ------ - nick\\n|true"}) {
		t.Fatalf("status=%v err=%v chats=%v", status, err, engine.chats)
	}

	repo.records = nil
	engine.chats = nil
	status, err = definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!banlist"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(engine.chats, []string{"mod|No users has been banned.|false"}) {
		t.Fatalf("status=%v err=%v chats=%v", status, err, engine.chats)
	}
}

func TestShadowBanSingleActivePersistsIdentityKicksAndReplies(t *testing.T) {
	repo := &shadowBanRepositoryStub{}
	engine := newShadowBanEngine(repo, map[string]*model.User{"merc": {Name: "merc", Trip: "trip", Hash: "hash"}})
	definition, _ := commandDefinitionFor("sban")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!sban @merc"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if got, want := repo.persisted, []repository.ShadowBanRecord{{Trip: "trip", Name: "merc", Hash: "hash"}}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("persisted=%+v want=%+v", got, want)
	}
	if !equalStrings(engine.kicked, []string{"merc"}) || !equalStrings(engine.chats, []string{"mod|shadow_banned: merc trip: trip hash: hash|false"}) {
		t.Fatalf("kicked=%v chats=%v", engine.kicked, engine.chats)
	}
}

func TestShadowBanSingleUsesAuthoritativeActiveIdentity(t *testing.T) {
	repo := &shadowBanRepositoryStub{}
	current := &model.User{Name: "merc", Trip: "current-trip", Hash: "current-hash"}
	engine := newShadowBanEngine(repo, map[string]*model.User{"Merc": current})
	definition, _ := commandDefinitionFor("shadowban")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban Merc"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if got, want := repo.persisted, []repository.ShadowBanRecord{{Trip: "current-trip", Name: "merc", Hash: "current-hash"}}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("persisted=%+v want=%+v", got, want)
	}
	if !equalStrings(engine.kicked, []string{"merc"}) || !equalStrings(engine.chats, []string{"mod|shadow_banned: Merc trip: current-trip hash: current-hash|false"}) {
		t.Fatalf("kicked=%v chats=%v", engine.kicked, engine.chats)
	}
}

func TestShadowBanOfflineAndContainsModePreserveSourceReplies(t *testing.T) {
	repo := &shadowBanRepositoryStub{}
	engine := newShadowBanEngine(repo, map[string]*model.User{
		"raider-a": {Name: "raider-a", Trip: "a", Hash: "one"},
		"other":    {Name: "other", Trip: "b", Hash: "two"},
	})
	definition, _ := commandDefinitionFor("shadowban")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban nobody"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(engine.chats, []string{"mod|banned: nobody|false"}) || len(repo.persisted) != 1 || repo.persisted[0] != (repository.ShadowBanRecord{Name: "nobody"}) {
		t.Fatalf("offline status=%v err=%v records=%+v chats=%v", status, err, repo.persisted, engine.chats)
	}

	repo.persisted, engine.chats, engine.kicked = nil, nil, nil
	status, err = definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban -c raid"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || len(repo.persisted) != 1 || repo.persisted[0].Name != "raider-a" || !equalStrings(engine.kicked, []string{"raider-a"}) || len(engine.chats) != 0 {
		t.Fatalf("contains status=%v err=%v records=%+v kicked=%v chats=%v", status, err, repo.persisted, engine.kicked, engine.chats)
	}
}

func TestShadowBanContainsUsesCurrentIdentityForSelectedActiveUser(t *testing.T) {
	repo := &shadowBanRepositoryStub{}
	stale := &model.User{Name: "raider", Trip: "stale-trip", Hash: "stale-hash"}
	current := &model.User{Name: "raider", Trip: "current-trip", Hash: "current-hash"}
	engine := newShadowBanEngine(repo, map[string]*model.User{"raider": stale})
	engine.activeBy = map[string]*model.User{"raider": current}
	definition, _ := commandDefinitionFor("shadowban")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban -c raid"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if got, want := repo.persisted, []repository.ShadowBanRecord{{Trip: "current-trip", Name: "raider", Hash: "current-hash"}}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("persisted=%+v want=%+v", got, want)
	}
	if !equalStrings(engine.kicked, []string{"raider"}) {
		t.Fatalf("kicked=%v", engine.kicked)
	}
}

func TestShadowBanSelectorsResolveUnicodeCanonicalNamesAndStopAfterPartialExit(t *testing.T) {
	definition, _ := commandDefinitionFor("shadowban")
	t.Run("exact Unicode mention", func(t *testing.T) {
		repo := &shadowBanRepositoryStub{}
		user := &model.User{Name: "Κόσμος", Trip: "exact-trip", Hash: "exact-hash"}
		engine := newShadowBanEngine(repo, map[string]*model.User{"canonical-key": user})
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban @ΚΌΣΜΟΣ"}).Execute(context.Background())
		if status != model.SUCCESSFUL || err != nil || len(repo.persisted) != 1 || repo.persisted[0].Name != "Κόσμος" || !equalStrings(engine.kicked, []string{"Κόσμος"}) {
			t.Fatalf("status=%v err=%v persisted=%+v kicked=%v", status, err, repo.persisted, engine.kicked)
		}
	})
	t.Run("contains cancellation after first kick", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		ctx, receipt := common.WithMutationRecorder(ctx)
		repo := &shadowBanRepositoryStub{}
		engine := newShadowBanEngine(repo, map[string]*model.User{
			"first":  {Name: "raid-a"},
			"second": {Name: "raid-b"},
		})
		engine.afterKick = cancel
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban -c raid"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, context.Canceled) || receipt.Count() != 1 || len(repo.persisted) != 1 || !equalStrings(engine.kicked, []string{"raid-a"}) {
			t.Fatalf("status=%v err=%v receipt=%d persisted=%+v kicked=%v", status, err, receipt.Count(), repo.persisted, engine.kicked)
		}
	})
	t.Run("cancellation after exact lookup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		repo := &shadowBanRepositoryStub{}
		engine := newShadowBanEngine(repo, map[string]*model.User{"target": {Name: "target"}})
		engine.afterLookup = cancel
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban target"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, context.Canceled) || len(repo.persisted) != 0 || len(engine.kicked) != 0 {
			t.Fatalf("status=%v err=%v persisted=%+v kicked=%v", status, err, repo.persisted, engine.kicked)
		}
	})
	t.Run("contains later kick failure", func(t *testing.T) {
		sendErr := errors.New("second shadow kick failed")
		ctx, receipt := common.WithMutationRecorder(context.Background())
		repo := &shadowBanRepositoryStub{}
		engine := newShadowBanEngine(repo, map[string]*model.User{
			"first":  {Name: "raid-a"},
			"second": {Name: "raid-b"},
		})
		engine.kickErrors = []error{nil, sendErr}
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban -c raid"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, sendErr) || receipt.Count() != 2 || len(repo.persisted) != 2 || !equalStrings(engine.kicked, []string{"raid-a", "raid-b"}) {
			t.Fatalf("status=%v err=%v receipt=%d persisted=%+v kicked=%v", status, err, receipt.Count(), repo.persisted, engine.kicked)
		}
	})
}

func TestShadowBanNormalizesRawInputOnceAndPreservesSourceCanonicalNames(t *testing.T) {
	definition, _ := commandDefinitionFor("shadowban")
	t.Run("double marker resolves literal at-name", func(t *testing.T) {
		repo := &shadowBanRepositoryStub{}
		literal := &model.User{Name: "@alice", Trip: "literal-trip", Hash: "literal-hash"}
		plain := &model.User{Name: "alice", Trip: "plain-trip", Hash: "plain-hash"}
		engine := newShadowBanEngine(repo, map[string]*model.User{"@alice": literal, "alice": plain})
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban @@alice"}).Execute(context.Background())
		if status != model.SUCCESSFUL || err != nil || len(repo.persisted) != 1 || repo.persisted[0].Name != "@alice" || !equalStrings(engine.kicked, []string{"@alice"}) {
			t.Fatalf("status=%v err=%v persisted=%+v kicked=%v", status, err, repo.persisted, engine.kicked)
		}
	})
	t.Run("contains keeps distinct source names", func(t *testing.T) {
		repo := &shadowBanRepositoryStub{}
		engine := newShadowBanEngine(repo, map[string]*model.User{
			"plain":   {Name: "alice", Trip: "plain-trip"},
			"literal": {Name: "@alice", Trip: "literal-trip"},
		})
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban -c alice"}).Execute(context.Background())
		if status != model.SUCCESSFUL || err != nil || len(repo.persisted) != 2 || repo.persisted[0].Name != "@alice" || repo.persisted[1].Name != "alice" || !equalStrings(engine.kicked, []string{"@alice", "alice"}) {
			t.Fatalf("status=%v err=%v persisted=%+v kicked=%v", status, err, repo.persisted, engine.kicked)
		}
	})
}

func TestUnshadowBanSingleAndAllDeleteUseRepositoryAndNoAgentPath(t *testing.T) {
	repo := &shadowBanRepositoryStub{records: []repository.ShadowBanRecord{{Hash: "hash", Trip: "trip", Name: "nick"}}, removeCount: 1, removeAllCount: 3}
	engine := newShadowBanEngine(repo, nil)
	definition, _ := commandDefinitionFor("unblock")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!unblock nick", IsWhisper: true}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || repo.removedTarget != "nick" || !equalStrings(engine.chats, []string{"mod|Unbanned shadow-ban records: 1|true"}) {
		t.Fatalf("single status=%v err=%v target=%q chats=%v", status, err, repo.removedTarget, engine.chats)
	}

	engine.chats = nil
	status, err = definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!unblock -all"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || repo.removeAlls != 1 || repo.listCalls != 0 || !equalStrings(engine.chats, []string{"mod|Unbanned shadow-ban records: 3|false"}) {
		t.Fatalf("all status=%v err=%v deleted=%d chats=%v", status, err, repo.removeAlls, engine.chats)
	}
}

func TestShadowBanRepositoryFailureDoesNotKickOrReply(t *testing.T) {
	repo := &shadowBanRepositoryStub{err: errors.New("db unavailable")}
	engine := newShadowBanEngine(repo, map[string]*model.User{"merc": {Name: "merc"}})
	definition, _ := commandDefinitionFor("shadowban")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban merc"}).Execute(context.Background())
	if status != model.FAILED || err == nil || len(engine.kicked) != 0 || len(engine.chats) != 0 {
		t.Fatalf("status=%v err=%v kicked=%v chats=%v", status, err, engine.kicked, engine.chats)
	}
}

func TestShadowBanCancellationAfterPersistDoesNotKickOrReply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &shadowBanRepositoryStub{afterPersist: cancel}
	engine := newShadowBanEngine(repo, map[string]*model.User{"merc": {Name: "merc"}})
	definition, _ := commandDefinitionFor("shadowban")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban merc"}).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, context.Canceled) || !equalStrings(engine.kicked, nil) || !equalStrings(engine.chats, nil) {
		t.Fatalf("status=%v err=%v kicked=%v chats=%v", status, err, engine.kicked, engine.chats)
	}
}

func TestShadowBanCanceledContextHasNoSideEffects(t *testing.T) {
	repo := &shadowBanRepositoryStub{}
	engine := newShadowBanEngine(repo, map[string]*model.User{"merc": {Name: "merc"}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	definition, _ := commandDefinitionFor("shadowban")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!shadowban merc"}).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, context.Canceled) || len(repo.persisted) != 0 || len(engine.kicked) != 0 || len(engine.chats) != 0 {
		t.Fatalf("status=%v err=%v records=%v kicked=%v chats=%v", status, err, repo.persisted, engine.kicked, engine.chats)
	}
}
