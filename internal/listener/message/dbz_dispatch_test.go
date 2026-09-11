package message_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/command"
	"zenbot/internal/common"
	message "zenbot/internal/listener/message"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/repository/sqlite"
	"zenbot/internal/service"
	"zenbot/internal/testutil/sqlitefixture"
)

type recordedDBZReply struct {
	recipient string
	text      string
	whisper   bool
}

type recordingDBZRepository struct {
	repository.DBZRepository
	statsReads     int
	freeStatsReads int
	strengthAdds   int
	registerCalls  int
	levelUpCalls   int
}

func (r *recordingDBZRepository) LevelUp(ctx context.Context, name string) error {
	r.levelUpCalls++
	return r.DBZRepository.LevelUp(ctx, name)
}

func (r *recordingDBZRepository) RegisterCharacter(ctx context.Context, name string, now func() int64) (int64, error) {
	r.registerCalls++
	return r.DBZRepository.RegisterCharacter(ctx, name, now)
}

func (r *recordingDBZRepository) Stats(ctx context.Context, name string) (repository.DBZStats, bool, error) {
	r.statsReads++
	return r.DBZRepository.Stats(ctx, name)
}

func (r *recordingDBZRepository) FreeStats(ctx context.Context, name string) (int, bool, error) {
	r.freeStatsReads++
	return r.DBZRepository.FreeStats(ctx, name)
}

func (r *recordingDBZRepository) AddStrength(ctx context.Context, name string, amount int) error {
	r.strengthAdds++
	return r.DBZRepository.AddStrength(ctx, name, amount)
}

type dbzDispatchEngine struct {
	bundle   *service.Bundle
	commands map[string]common.CommandMetadata
	allowed  map[model.Role]bool
	replies  []recordedDBZReply
	active   map[*model.User]struct{}
}

func (e *dbzDispatchEngine) ServiceBundle() *service.Bundle { return e.bundle }
func (e *dbzDispatchEngine) Start()                         { panic("unexpected engine Start") }
func (e *dbzDispatchEngine) Stop()                          { panic("unexpected engine Stop") }
func (e *dbzDispatchEngine) DispatchMessage(string)         { panic("unexpected engine DispatchMessage") }
func (e *dbzDispatchEngine) SendRawMessage(string)          { panic("unexpected engine SendRawMessage") }
func (e *dbzDispatchEngine) GetPrefix() string              { return "!" }
func (e *dbzDispatchEngine) GetEnabledCommands() *map[string]common.CommandMetadata {
	return &e.commands
}
func (e *dbzDispatchEngine) SendWhisperMessage(string, string) (string, error) {
	panic("unexpected engine SendWhisperMessage")
}
func (e *dbzDispatchEngine) SendAddressedMessage(string, string, bool) (string, error) {
	panic("unexpected engine SendAddressedMessage")
}
func (e *dbzDispatchEngine) AddActiveUser(*model.User) { panic("unexpected engine AddActiveUser") }
func (e *dbzDispatchEngine) RemoveActiveUser(*model.User) {
	panic("unexpected engine RemoveActiveUser")
}
func (e *dbzDispatchEngine) SubscribeTrip(string) bool { panic("unexpected engine SubscribeTrip") }
func (e *dbzDispatchEngine) UnsubscribeTrip(string) bool {
	panic("unexpected engine UnsubscribeTrip")
}
func (e *dbzDispatchEngine) IsSubscribedTrip(string) bool {
	panic("unexpected engine IsSubscribedTrip")
}
func (e *dbzDispatchEngine) GetSubscribedTrips() []string {
	panic("unexpected engine GetSubscribedTrips")
}
func (e *dbzDispatchEngine) AddAfkUser(*model.User, string) {
	panic("unexpected engine AddAfkUser")
}
func (e *dbzDispatchEngine) GetAfkUsers() *map[*model.User]string {
	panic("unexpected engine GetAfkUsers")
}
func (e *dbzDispatchEngine) GetActiveUserByName(string) *model.User {
	panic("unexpected engine GetActiveUserByName")
}
func (e *dbzDispatchEngine) Kick(string, string) { panic("unexpected engine Kick") }
func (e *dbzDispatchEngine) Ban(string)          { panic("unexpected engine Ban") }
func (e *dbzDispatchEngine) Unban(string)        { panic("unexpected engine Unban") }
func (e *dbzDispatchEngine) UnbanAll()           { panic("unexpected engine UnbanAll") }
func (e *dbzDispatchEngine) Lock()               { panic("unexpected engine Lock") }
func (e *dbzDispatchEngine) Unlock()             { panic("unexpected engine Unlock") }
func (e *dbzDispatchEngine) GetName() string     { panic("unexpected engine GetName") }
func (e *dbzDispatchEngine) GetChannel() string  { panic("unexpected engine GetChannel") }
func (e *dbzDispatchEngine) SetName(string)      { panic("unexpected engine SetName") }
func (e *dbzDispatchEngine) SetOnlineSetListener(common.Listener) {
	panic("unexpected engine SetOnlineSetListener")
}
func (e *dbzDispatchEngine) WaitConnectionWgDone() { panic("unexpected engine WaitConnectionWgDone") }
func (e *dbzDispatchEngine) LogMessage(string, string, string, string, string) (int64, error) {
	panic("unexpected engine LogMessage")
}
func (e *dbzDispatchEngine) LogPresence(string, string, string, string, string) (int64, error) {
	panic("unexpected engine LogPresence")
}
func (e *dbzDispatchEngine) SetLastKickedUser(string) {
	panic("unexpected engine SetLastKickedUser")
}
func (e *dbzDispatchEngine) SetLastKickedChannel(string) {
	panic("unexpected engine SetLastKickedChannel")
}
func (e *dbzDispatchEngine) NotifyAfkIfMentioned(*model.ChatMessage) {
	panic("unexpected engine NotifyAfkIfMentioned")
}
func (e *dbzDispatchEngine) RemoveIfAfk(*model.User) { panic("unexpected engine RemoveIfAfk") }
func (e *dbzDispatchEngine) RegisterCommand(c common.Command) error {
	for _, alias := range c.GetAliases() {
		registered := c
		e.commands[strings.ToLower(alias)] = common.CommandMetadata{
			Alias: alias,
			Command: func(message *model.ChatMessage) common.Command {
				return registered.NewInstance(e, message)
			},
		}
	}
	return nil
}
func (e *dbzDispatchEngine) IsUserAuthorized(_ *model.User, role *model.Role) bool {
	return role != nil && e.allowed[*role]
}
func (e *dbzDispatchEngine) SendChatMessage(recipient, text string, whisper bool) (string, error) {
	e.replies = append(e.replies, recordedDBZReply{recipient: recipient, text: text, whisper: whisper})
	return "", nil
}
func (e *dbzDispatchEngine) GetActiveUsers() *map[*model.User]struct{} { return &e.active }

func openDBZDispatchTestDB(t *testing.T) *sqlite.Database {
	t.Helper()
	return sqlitefixture.Open(t, "dbz-dispatch")
}

func seedDBZDispatchSnapshot(t *testing.T, database *sqlite.Database, name string) {
	t.Helper()
	ctx := context.Background()
	if _, err := database.DB.ExecContext(ctx, "INSERT INTO dbz_characters(name,level,created_on) VALUES(?1,?2,?3)", name, 2, 1); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := database.DB.QueryRowContext(ctx, "SELECT id FROM dbz_characters WHERE name=?1", name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, "INSERT INTO dbz_stats(char_id,free_stats,str,agi,vit,ene,created_on) VALUES(?1,?2,?3,?4,?5,?6,?7)", id, 5, 3, 4, 5, 6, 1); err != nil {
		t.Fatal(err)
	}
}

func newDBZDispatchEngine(database *sqlite.Database, allowed map[model.Role]bool) *dbzDispatchEngine {
	return &dbzDispatchEngine{
		bundle:   &service.Bundle{DBZ: &service.DBZService{Repo: database}},
		commands: map[string]common.CommandMetadata{},
		allowed:  allowed,
		active:   map[*model.User]struct{}{},
	}
}

func TestDispatchUserCommandDBZStatsAuthorizedWhisperUsesCallerNameAndExactStatsReply(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	_, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
		Engine:  engine,
		Message: &model.ChatMessage{Name: "goku", Text: "!ds", IsWhisper: true},
		Author:  &model.User{Name: "goku"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(engine.replies) != 1 {
		t.Fatalf("replies=%d, want exactly one: %#v", len(engine.replies), engine.replies)
	}
	got := engine.replies[0]
	want := "character: goku\nlevel: 2\nfree stats: 5\nstr: 3\nagi: 4\nvit: 5\nene: 6\n"
	if got.recipient != "goku" || got.text != want || !got.whisper {
		t.Fatalf("reply=%#v, want recipient goku, exact stats payload, whisper true", got)
	}
}

func TestDispatchUserCommandDBZStatsCanonicalReadsCallerAndRepliesExactText(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	_, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
		Engine:  engine,
		Message: &model.ChatMessage{Name: "goku", Text: "!dbzstats", IsWhisper: true},
		Author:  &model.User{Name: "goku"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(engine.replies) != 1 {
		t.Fatalf("replies=%d, want exactly one: %#v", len(engine.replies), engine.replies)
	}
	got := engine.replies[0]
	want := "character: goku\nlevel: 2\nfree stats: 5\nstr: 3\nagi: 4\nvit: 5\nene: 6\n"
	if got.recipient != "goku" || got.text != want || !got.whisper {
		t.Fatalf("reply=%#v, want recipient goku, exact stats payload, whisper true", got)
	}
}

func TestDispatchUserCommandDBZStatsMissingCharacterDoesNotPublishSuccess(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	_, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
		Engine:  engine,
		Message: &model.ChatMessage{Name: "missing", Text: "!dbzstats", IsWhisper: true},
		Author:  &model.User{Name: "missing"},
	})
	if err == nil {
		t.Fatal("dispatch lost the command failure")
	}
	if len(engine.replies) != 0 {
		t.Fatalf("replies=%#v, want no success output", engine.replies)
	}
}

func TestDispatchUserCommandDBZStatsDeniedRegularDoesNotReadAndUsesSharedUnauthorizedReply(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	repo := &recordingDBZRepository{DBZRepository: database}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.USER: true, model.REGULAR: false})
	engine.bundle.DBZ.Repo = repo
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	state := &message.Context{Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!ds", IsWhisper: true}}
	if _, err := (message.ResolveUserMetadata{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if state.Author != user {
		t.Fatalf("resolved author=%#v, want active goku", state.Author)
	}
	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	if repo.statsReads != 0 {
		t.Fatalf("Stats reads=%d, want authorization to stop before DBZ read", repo.statsReads)
	}
	if len(engine.replies) != 1 {
		t.Fatalf("replies=%d, want one shared unauthorized reply: %#v", len(engine.replies), engine.replies)
	}
	got := engine.replies[0]
	want := " you are not authorized to run: ds command."
	if got.recipient != "goku" || got.text != want || !got.whisper {
		t.Fatalf("reply=%#v, want shared unauthorized reply whispered to resolved goku", got)
	}
}

func TestDispatchUserCommandDBZRegisterDeniedRegularDoesNotPersistAndUsesSharedUnauthorizedReply(t *testing.T) {
	// Regression/provenance limitation: existing shared dispatch authorization
	// already denies this route before its DBZ repository seam.
	database := openDBZDispatchTestDB(t)
	repo := &recordingDBZRepository{DBZRepository: database}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.USER: true, model.REGULAR: false})
	engine.bundle.DBZ.Repo = repo
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	state := &message.Context{Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dr", IsWhisper: true}}
	if _, err := (message.ResolveUserMetadata{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if repo.registerCalls != 0 {
		t.Fatalf("RegisterCharacter calls=%d, want authorization to stop before DBZ write", repo.registerCalls)
	}
	var characters int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM dbz_characters WHERE name=?1", "goku").Scan(&characters); err != nil {
		t.Fatal(err)
	}
	if characters != 0 {
		t.Fatalf("characters=%d, want no DBZ write", characters)
	}
	if len(engine.replies) != 1 {
		t.Fatalf("replies=%d, want one shared unauthorized reply: %#v", len(engine.replies), engine.replies)
	}
	got := engine.replies[0]
	if got.recipient != "goku" || got.text != " you are not authorized to run: dr command." || !got.whisper {
		t.Fatalf("reply=%#v, want shared unauthorized reply whispered to resolved goku", got)
	}
}

func TestDispatchUserCommandDBZRegisterPreCancelledDoesNotPersistOrReply(t *testing.T) {
	// Regression/provenance limitation: dbzCommand's existing pre-persistence
	// context check makes this direct DBZ route green on its first run.
	database := openDBZDispatchTestDB(t)
	repo := &recordingDBZRepository{DBZRepository: database}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	engine.bundle.DBZ.Repo = repo
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	state := &message.Context{Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dbzregister", IsWhisper: true}}
	if _, err := (message.ResolveUserMetadata{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (message.DispatchUserCommand{}).Handle(ctx, state); !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatch error=%v, want cancellation", err)
	}
	if repo.registerCalls != 0 || len(engine.replies) != 0 {
		t.Fatalf("RegisterCharacter calls=%d replies=%#v, want no write and no output", repo.registerCalls, engine.replies)
	}
	var characters int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM dbz_characters WHERE name=?1", "goku").Scan(&characters); err != nil {
		t.Fatal(err)
	}
	if characters != 0 {
		t.Fatalf("characters=%d, want no DBZ write", characters)
	}
}

func TestDispatchUserCommandDBZRegisterAliasesPersistSelfAndPublishPublicSuccess(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	state := &message.Context{Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dreg ignored tokens", IsWhisper: true}}
	if _, err := (message.ResolveUserMetadata{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if state.Author != user {
		t.Fatalf("resolved author=%#v, want active goku", state.Author)
	}
	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	var characters, ignored, stats int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM dbz_characters WHERE name=?1 AND level=?2", "goku", 1).Scan(&characters); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM dbz_characters WHERE name=?1", "ignored").Scan(&ignored); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM dbz_stats s JOIN dbz_characters c ON c.id=s.char_id WHERE c.name=?1 AND s.free_stats=?2 AND s.str=?3 AND s.agi=?4 AND s.vit=?5 AND s.ene=?6`, "goku", 0, 1, 1, 1, 1).Scan(&stats); err != nil {
		t.Fatal(err)
	}
	if characters != 1 || ignored != 0 || stats != 1 {
		t.Fatalf("characters=%d ignored=%d stats=%d", characters, ignored, stats)
	}
	if len(engine.replies) != 1 {
		t.Fatalf("replies=%d, want exactly one: %#v", len(engine.replies), engine.replies)
	}
	got := engine.replies[0]
	if got.recipient != "" || got.text != "Successfully registered character: goku" || got.whisper {
		t.Fatalf("reply=%#v, want public exact registration acknowledgement", got)
	}
}

func TestDispatchUserCommandDBZRegisterFailureRollsBackAndPublishesNoSuccess(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	if _, err := database.DB.Exec(`CREATE TRIGGER reject_dispatch_initial_stats BEFORE INSERT ON dbz_stats WHEN NEW.str <= 1 BEGIN SELECT RAISE(ABORT, 'rejected stats'); END`); err != nil {
		t.Fatal(err)
	}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
		Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dbzregister", IsWhisper: true}, Author: user,
	}); err == nil {
		t.Fatal("dispatch lost the command failure")
	}
	var characters, stats int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM dbz_characters WHERE name=?1", "goku").Scan(&characters); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM dbz_stats").Scan(&stats); err != nil {
		t.Fatal(err)
	}
	if characters != 0 || stats != 0 || len(engine.replies) != 0 {
		t.Fatalf("characters=%d stats=%d replies=%#v", characters, stats, engine.replies)
	}
}

func TestDispatchUserCommandDBZStrengthAliasRejectsOverspendWithoutSuccessOutput(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	if _, err := database.DB.Exec("UPDATE dbz_stats SET free_stats=?1 WHERE char_id=(SELECT id FROM dbz_characters WHERE name=?2)", 1, "goku"); err != nil {
		t.Fatal(err)
	}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	_, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
		Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!daddstr 2 ignored", IsWhisper: true}, Author: user,
	})
	if err == nil {
		t.Fatal("dispatch lost the command failure")
	}
	if len(engine.replies) != 0 {
		t.Fatalf("replies=%#v, want no success output", engine.replies)
	}
	var strength, free int
	if err := database.DB.QueryRow("SELECT str,free_stats FROM dbz_stats WHERE char_id=(SELECT id FROM dbz_characters WHERE name=?1)", "goku").Scan(&strength, &free); err != nil {
		t.Fatal(err)
	}
	if strength != 3 || free != 1 {
		t.Fatalf("strength=%d free_stats=%d, want unchanged 3 1", strength, free)
	}
}

func TestDispatchUserCommandDBZStrengthSpendsExactBalanceBeforeAcknowledgement(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
		Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dstr 2", IsWhisper: true}, Author: user,
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := engine.replies, []recordedDBZReply{{recipient: "", text: "2", whisper: false}}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("replies=%#v, want %#v", got, want)
	}
	var strength, free int
	if err := database.DB.QueryRow("SELECT str,free_stats FROM dbz_stats WHERE char_id=(SELECT id FROM dbz_characters WHERE name=?1)", "goku").Scan(&strength, &free); err != nil {
		t.Fatal(err)
	}
	if strength != 5 || free != 3 {
		t.Fatalf("strength=%d free_stats=%d, want 5 3", strength, free)
	}
}

func TestDispatchUserCommandDBZStrengthInvalidAmountRepliesUsageWithoutMutation(t *testing.T) {
	for _, input := range []string{"", "0", "-1", "nope", "2147483648"} {
		t.Run(input, func(t *testing.T) {
			database := openDBZDispatchTestDB(t)
			seedDBZDispatchSnapshot(t, database, "goku")
			engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
			user := &model.User{Name: "goku"}
			engine.active[user] = struct{}{}
			if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
				t.Fatal(err)
			}
			_, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
				Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!daddstr " + input, IsWhisper: true}, Author: user,
			})
			var rejection *common.CommandRejectedError
			if !errors.As(err, &rejection) || rejection.Status != model.FAILED {
				t.Fatalf("dispatch error=%v, want command rejection", err)
			}
			if got, want := engine.replies, []recordedDBZReply{{recipient: "goku", text: "Example: !daddstr amount", whisper: true}}; len(got) != 1 || got[0] != want[0] {
				t.Fatalf("replies=%#v, want %#v", got, want)
			}
			var strength, free int
			if err := database.DB.QueryRow("SELECT str,free_stats FROM dbz_stats WHERE char_id=(SELECT id FROM dbz_characters WHERE name=?1)", "goku").Scan(&strength, &free); err != nil {
				t.Fatal(err)
			}
			if strength != 3 || free != 5 {
				t.Fatalf("strength=%d free_stats=%d, want unchanged 3 5", strength, free)
			}
		})
	}
}

func TestDispatchUserCommandDBZStrengthNoFreeStatsDoesNotPublishSuccess(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	if _, err := database.DB.Exec("UPDATE dbz_stats SET free_stats=?1 WHERE char_id=(SELECT id FROM dbz_characters WHERE name=?2)", 0, "goku"); err != nil {
		t.Fatal(err)
	}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dstr 1", IsWhisper: true}, Author: user}); err == nil {
		t.Fatal("dispatch lost the command failure")
	}
	if len(engine.replies) != 0 {
		t.Fatalf("replies=%#v, want no success output", engine.replies)
	}
	var strength, free int
	if err := database.DB.QueryRow("SELECT str,free_stats FROM dbz_stats WHERE char_id=(SELECT id FROM dbz_characters WHERE name=?1)", "goku").Scan(&strength, &free); err != nil {
		t.Fatal(err)
	}
	if strength != 3 || free != 0 {
		t.Fatalf("strength=%d free_stats=%d, want unchanged 3 0", strength, free)
	}
}

func TestDispatchUserCommandDBZStrengthDeniedRegularDoesNotReadOrWrite(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	repo := &recordingDBZRepository{DBZRepository: database}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.USER: true, model.REGULAR: false})
	engine.bundle.DBZ.Repo = repo
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}
	state := &message.Context{Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dstr 1", IsWhisper: true}}
	if _, err := (message.ResolveUserMetadata{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if repo.freeStatsReads != 0 || repo.strengthAdds != 0 {
		t.Fatalf("FreeStats=%d AddStrength=%d, want no DBZ calls", repo.freeStatsReads, repo.strengthAdds)
	}
	if got, want := engine.replies, []recordedDBZReply{{recipient: "goku", text: " you are not authorized to run: dstr command.", whisper: true}}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("replies=%#v want=%#v", got, want)
	}
}

func TestDispatchUserCommandDBZStrengthPreCancelledDoesNotReadWriteOrReply(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	repo := &recordingDBZRepository{DBZRepository: database}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	engine.bundle.DBZ.Repo = repo
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (message.DispatchUserCommand{}).Handle(ctx, &message.Context{Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dstr 1", IsWhisper: true}, Author: user})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatch error=%v, want cancellation", err)
	}
	if repo.freeStatsReads != 0 || repo.strengthAdds != 0 || len(engine.replies) != 0 {
		t.Fatalf("FreeStats=%d AddStrength=%d replies=%#v, want no DBZ calls/output", repo.freeStatsReads, repo.strengthAdds, engine.replies)
	}
}

func TestDispatchUserCommandDBZFightAliasConsumesFirstMatchLevelsWithoutEnemyProofAndPublishes(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	engine.bundle.DBZ.SpawnEnemy("frieza")
	engine.bundle.DBZ.SpawnEnemy("frieza")
	engine.bundle.DBZ.SpawnEnemy("cell")
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
		Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!df frieza ignored", IsWhisper: true}, Author: user,
	}); err != nil {
		t.Fatal(err)
	}

	if got, want := engine.replies, []recordedDBZReply{{recipient: "", text: "Gz. Enemy has been slain. Your leveled up! Granted 5 free stats!", whisper: false}}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("replies=%#v, want %#v", got, want)
	}
	if got, want := engine.bundle.DBZ.Enemies(), []string{"frieza", "cell"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("enemies=%q, want %q", got, want)
	}
	var level, freeStats int
	if err := database.DB.QueryRow("SELECT c.level,s.free_stats FROM dbz_characters c JOIN dbz_stats s ON s.char_id=c.id WHERE c.name=?1", "goku").Scan(&level, &freeStats); err != nil {
		t.Fatal(err)
	}
	if level != 3 || freeStats != 10 {
		t.Fatalf("level=%d free_stats=%d, want 3 10", level, freeStats)
	}
}

func TestDispatchUserCommandDBZFightMissingEnemyDoesNotRewardOrPublishSuccess(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
		Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dfight absent", IsWhisper: true}, Author: user,
	}); err == nil {
		t.Fatal("dispatch lost the command failure")
	}
	if len(engine.replies) != 0 {
		t.Fatalf("replies=%#v, want no success output", engine.replies)
	}
	if enemies := engine.bundle.DBZ.Enemies(); len(enemies) != 0 {
		t.Fatalf("enemies=%q, want empty", enemies)
	}
	var level, freeStats int
	if err := database.DB.QueryRow("SELECT c.level,s.free_stats FROM dbz_characters c JOIN dbz_stats s ON s.char_id=c.id WHERE c.name=?1", "goku").Scan(&level, &freeStats); err != nil {
		t.Fatal(err)
	}
	if level != 2 || freeStats != 5 {
		t.Fatalf("level=%d free_stats=%d, want unchanged 2 5", level, freeStats)
	}
}

func TestDispatchUserCommandDBZFightMissingEnemyArgumentUsesSourceUsageWithoutMutation(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	engine.bundle.DBZ.SpawnEnemy("frieza")
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}

	_, err := (message.DispatchUserCommand{}).Handle(context.Background(), &message.Context{
		Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dfight   ", IsWhisper: true}, Author: user,
	})
	var rejection *common.CommandRejectedError
	if !errors.As(err, &rejection) || rejection.Status != model.FAILED {
		t.Fatalf("dispatch error=%v, want command rejection", err)
	}
	if got, want := engine.replies, []recordedDBZReply{{recipient: "goku", text: "Example: !dfight enemy", whisper: true}}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("replies=%#v, want %#v", got, want)
	}
	if got, want := engine.bundle.DBZ.Enemies(), []string{"frieza"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("enemies=%q, want %q", got, want)
	}
	var level, freeStats int
	if err := database.DB.QueryRow("SELECT c.level,s.free_stats FROM dbz_characters c JOIN dbz_stats s ON s.char_id=c.id WHERE c.name=?1", "goku").Scan(&level, &freeStats); err != nil {
		t.Fatal(err)
	}
	if level != 2 || freeStats != 5 {
		t.Fatalf("level=%d free_stats=%d, want unchanged 2 5", level, freeStats)
	}
}

func TestDispatchUserCommandDBZFightDeniedRegularHasNoFightOrLevelUpEffects(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	repo := &recordingDBZRepository{DBZRepository: database}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.USER: true, model.REGULAR: false})
	engine.bundle.DBZ.Repo = repo
	engine.bundle.DBZ.SpawnEnemy("frieza")
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}
	state := &message.Context{Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dfight frieza", IsWhisper: true}}
	if _, err := (message.ResolveUserMetadata{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if _, err := (message.DispatchUserCommand{}).Handle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if repo.levelUpCalls != 0 {
		t.Fatalf("LevelUp calls=%d, want no authorization-bypassing write", repo.levelUpCalls)
	}
	if got, want := engine.bundle.DBZ.Enemies(), []string{"frieza"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("enemies=%q, want %q", got, want)
	}
	if got, want := engine.replies, []recordedDBZReply{{recipient: "goku", text: " you are not authorized to run: dfight command.", whisper: true}}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("replies=%#v, want %#v", got, want)
	}
}

func TestDispatchUserCommandDBZFightPreCancelledHasNoFightOrLevelUpEffects(t *testing.T) {
	database := openDBZDispatchTestDB(t)
	seedDBZDispatchSnapshot(t, database, "goku")
	repo := &recordingDBZRepository{DBZRepository: database}
	engine := newDBZDispatchEngine(database, map[model.Role]bool{model.REGULAR: true})
	engine.bundle.DBZ.Repo = repo
	engine.bundle.DBZ.SpawnEnemy("frieza")
	user := &model.User{Name: "goku"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(engine, nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (message.DispatchUserCommand{}).Handle(ctx, &message.Context{Engine: engine, Message: &model.ChatMessage{Name: "goku", Text: "!dfight frieza", IsWhisper: true}, Author: user}); !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatch error=%v, want cancellation", err)
	}
	if repo.levelUpCalls != 0 || len(engine.replies) != 0 {
		t.Fatalf("LevelUp calls=%d replies=%#v, want no effects", repo.levelUpCalls, engine.replies)
	}
	if got, want := engine.bundle.DBZ.Enemies(), []string{"frieza"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("enemies=%q, want %q", got, want)
	}
}
