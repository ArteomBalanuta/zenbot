package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

func TestTextObservationAuditPingFactSurvivesFailedDelivery(t *testing.T) {
	engine := &gatewayEngine{authorized: true, sendErr: errors.New("private send diagnostic")}
	engine.bundle = &service.Bundle{Ping: auditPingService(t)}
	caller, err := api.NewContext("room", "caller", "trip", "hash", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := commandcatalog.AgentEntry("ping")
	if !ok {
		t.Fatal("ping catalog entry absent")
	}
	commandTool := tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}
	result, err := commandTool.Execute(context.Background(), caller, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || result.DeliveryCount != 0 || engine.sends != 1 {
		t.Fatalf("fixture did not fail after source measurement: result=%+v sends=%d", result, engine.sends)
	}
	envelope := string(result.Envelope())
	if !strings.Contains(envelope, "response time:") || !strings.Contains(envelope, "milliseconds") {
		t.Errorf("completed source measurement was lost after failed delivery: %s", envelope)
	}
	if strings.Contains(envelope, "private send diagnostic") {
		t.Errorf("raw delivery diagnostic was exposed: %s", envelope)
	}
}

func TestTextObservationAuditForcedPrivateHelpUsesActualVisibility(t *testing.T) {
	for _, private := range []bool{false, true} {
		t.Run(map[bool]string{false: "public invocation", true: "private invocation"}[private], func(t *testing.T) {
			engine := &capturePrivacyFailingEngine{
				commandEngineStub: &commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}},
				err:               errors.New("private help send diagnostic"),
			}
			caller, err := api.NewContext("room", "caller", "trip", "hash", private, []string{})
			if err != nil {
				t.Fatal(err)
			}
			entry, _ := commandcatalog.AgentEntry("help")
			result, err := (tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}).Execute(context.Background(), caller, []byte(`{}`))
			if err != nil || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.DeliveryCount != 0 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			envelope := string(result.Envelope())
			if strings.Contains(envelope, "private help send diagnostic") {
				t.Fatalf("raw diagnostic leaked: %s", envelope)
			}
			containsHelp := strings.Contains(envelope, "Moderator commands")
			if containsHelp != private {
				t.Fatalf("private=%v envelope=%s", private, envelope)
			}
		})
	}
}

func TestTextObservationAuditForcedPrivateNotesUseActualVisibility(t *testing.T) {
	database := h2fixture.Open(t, "text-observation-private-notes")
	if err := (&service.NoteService{DB: database.DB}).Save(context.Background(), "trip", "secret 🍵"); err != nil {
		t.Fatal(err)
	}
	for _, private := range []bool{false, true} {
		t.Run(map[bool]string{false: "public invocation", true: "private invocation"}[private], func(t *testing.T) {
			engine := &capturePrivacyFailingEngine{
				commandEngineStub: &commandEngineStub{
					users:  map[string]*model.User{"caller": {Name: "caller", Trip: "trip"}},
					bundle: &service.Bundle{Notes: &service.NoteService{DB: database.DB}},
				},
				err: errors.New("private notes send diagnostic"),
			}
			caller, err := api.NewContext("room", "caller", "trip", "hash", private, []string{})
			if err != nil {
				t.Fatal(err)
			}
			entry, _ := commandcatalog.AgentEntry("notes")
			result, err := (tool.SaturnCommand{Definition: entry, Gateway: NewAgentCommandGateway(engine)}).Execute(context.Background(), caller, []byte(`{"operation":"list"}`))
			if err != nil || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.DeliveryCount != 0 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			envelope := string(result.Envelope())
			containsSecret := strings.Contains(envelope, "secret 🍵")
			if containsSecret != private {
				t.Fatalf("private=%v envelope=%s", private, envelope)
			}
			if strings.Contains(envelope, "private notes send diagnostic") {
				t.Fatalf("raw diagnostic leaked: %s", envelope)
			}
		})
	}
}

type textObservationFailingEngine struct {
	common.Engine
	sendErr  error
	attempts int
}

func (e *textObservationFailingEngine) ServiceBundle() *service.Bundle {
	return bundle(e.Engine)
}

func (e *textObservationFailingEngine) SendChatMessage(string, string, bool) (string, error) {
	e.attempts++
	return "", e.sendErr
}

func (e *textObservationFailingEngine) SendWhisperMessage(string, string) (string, error) {
	e.attempts++
	return "", e.sendErr
}

func (e *textObservationFailingEngine) SendAddressedMessage(string, string, bool) (string, error) {
	e.attempts++
	return "", e.sendErr
}

func (e *textObservationFailingEngine) AddReplica(ctx context.Context, channel string) error {
	return e.Engine.(ReplicaController).AddReplica(ctx, channel)
}

func (e *textObservationFailingEngine) RemoveReplica(ctx context.Context, channel string) error {
	return e.Engine.(ReplicaController).RemoveReplica(ctx, channel)
}

func (e *textObservationFailingEngine) ReplicaChannels() []string {
	return e.Engine.(ReplicaController).ReplicaChannels()
}

func executeTextObservationSource(t *testing.T, engine common.Engine, canonical, commandLine string, trip string) string {
	t.Helper()
	wantErr := errors.New("source output delivery failed")
	failing := &textObservationFailingEngine{Engine: engine, sendErr: wantErr}
	capture := &agentCaptureEngine{Engine: failing, invocationWhisper: true}
	definition, ok := commandDefinitionFor(canonical)
	if !ok {
		t.Fatalf("missing command %q", canonical)
	}
	status, err := definition.New(capture, &model.ChatMessage{Name: "caller", Trip: trip, Text: commandLine, IsWhisper: true}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, wantErr) || failing.attempts != 1 || !capture.dataObserved {
		t.Fatalf("%s status=%s err=%v attempts=%d observed=%v data=%s", canonical, status, err, failing.attempts, capture.dataObserved, capture.data)
	}
	var observation commandTextObservation
	if err := json.Unmarshal(capture.data, &observation); err != nil {
		t.Fatalf("%s data=%s err=%v", canonical, capture.data, err)
	}
	return observation.Text
}

func TestTextObservationAuditSourceReadInventoryRunsOnceAndRetainsText(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = connection.Close()
		}
	}()
	server := auditUtilityServer(t)
	utilities := &commandEngineStub{bundle: &service.Bundle{
		Ping:    &service.PingService{Address: listener.Addr().String()},
		Weather: &service.WeatherService{HTTP: server.Client(), GeoURL: server.URL + "/geo", ForecastURL: server.URL + "/forecast"},
		Time:    &service.TimeService{HTTP: server.Client(), GeoURL: server.URL + "/geo", SunriseURL: server.URL + "/sun?lat=%s&lng=%s", TimezoneURL: server.URL + "/time?lat=%s&lng=%s"},
	}}
	for _, tc := range []struct {
		canonical, commandLine, contains string
	}{
		{"ping", "!ping", "response time:"},
		{"weather", "!weather City", "21"},
		{"time", "!time City", "12:00"},
	} {
		if got := executeTextObservationSource(t, utilities, tc.canonical, tc.commandLine, ""); !strings.Contains(got, tc.contains) {
			t.Fatalf("%s observation=%q", tc.canonical, got)
		}
	}

	queries := &lastOnlineCommandQueriesStub{record: repository.LastOnlineRecord{
		Found:          true,
		LastMessage:    sql.NullString{String: "hello", Valid: true},
		LastSeenMillis: sql.NullInt64{Int64: 0, Valid: true},
	}}
	users := &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{
		Queries: queries,
		Now:     func() time.Time { return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC) },
	}}}
	if got := executeTextObservationSource(t, users, "lastonline", "!lastonline merc", ""); !strings.Contains(got, "Last message: hello") || queries.calls != 1 {
		t.Fatalf("lastonline observation=%q calls=%d", got, queries.calls)
	}
	if got := executeTextObservationSource(t, users, "users", "!users", ""); !strings.Contains(got, "Users:") || queries.registeredCalls != 1 {
		t.Fatalf("users observation=%q calls=%d", got, queries.registeredCalls)
	}
	if got := executeTextObservationSource(t, users, "nicks", "!nicks unknown-trip", ""); got != "" || queries.nicksCalls != 1 {
		t.Fatalf("empty nicks observation=%q calls=%d", got, queries.nicksCalls)
	}

	activityRepo := &activityCommandRepositoryStub{stats: []repository.ActivityStat{{Trip: "trip", DayOfWeek: "Sunday", Hour: "1", ProbabilityPercentage: "100"}}}
	activity := &commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: activityRepo}}}
	if got := executeTextObservationSource(t, activity, "active", "!active trip", ""); !strings.Contains(got, "100") || activityRepo.calls != 1 {
		t.Fatalf("activity observation=%q calls=%d", got, activityRepo.calls)
	}

	historyRepo := &identityFake{messages: []model.Message{{Name: "alice", Trip: "trip", Message: "history fact"}}}
	history := newIdentityEngine(historyRepo, &authFake{})
	if got := executeTextObservationSource(t, history, "messages", "!messages trip 1", ""); !strings.Contains(got, "history fact") || historyRepo.lastCalls != 1 {
		t.Fatalf("history observation=%q calls=%d", got, historyRepo.lastCalls)
	}

	sqlSource := &recordingRawSQLQuery{}
	sqlEngine := &commandEngineStub{bundle: &service.Bundle{SQLCommand: sqlSource}}
	if got := executeTextObservationSource(t, sqlEngine, "sql", "!sql SELECT 1", ""); !strings.Contains(got, "Result:") || len(sqlSource.queries) != 1 {
		t.Fatalf("sql observation=%q queries=%v", got, sqlSource.queries)
	}
}

func TestTextObservationAuditStaticAndRuntimeInventoryRetainsText(t *testing.T) {
	standard := &commandEngineStub{users: map[string]*model.User{
		"target": {Name: "target", Trip: "trip", Hash: "hash"},
	}}
	for _, tc := range []struct {
		canonical, commandLine, contains string
	}{
		{"version", "!version", ""},
		{"ape", "!ape", "⣄"},
		{"coin", "!coin", ""},
		{"memory", "!memory", "Go Alloc:"},
		{"info", "!info target", "User hash: hash"},
		{"crashcourse", "!crashcourse", "moderation guide"},
	} {
		got := executeTextObservationSource(t, standard, tc.canonical, tc.commandLine, "")
		if tc.canonical == "coin" {
			if got != "head" && got != "tail" {
				t.Fatalf("coin observation=%q", got)
			}
		} else if tc.canonical == "version" {
			if got == "" {
				t.Fatal("version observation was empty")
			}
		} else if !strings.Contains(got, tc.contains) {
			t.Fatalf("%s observation=%q", tc.canonical, got)
		}
	}

	replicas := &replicaRegistrationEngine{commandEngineStub: &commandEngineStub{}, fakeRC: fakeRC{channels: []string{"z", "a"}}}
	if got := executeTextObservationSource(t, replicas, "replicastatus", "!replicastatus", ""); !strings.Contains(got, "Serving channels: a, z") {
		t.Fatalf("replica status observation=%q", got)
	}

	shadowRepo := &shadowBanRepositoryStub{records: []repository.ShadowBanRecord{{Hash: "hash", Trip: "trip", Name: "name"}}}
	shadow := &commandEngineStub{bundle: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: shadowRepo}}}
	if got := executeTextObservationSource(t, shadow, "shadowbanlist", "!shadowbanlist", ""); !strings.Contains(got, "hash - trip - name") || shadowRepo.listCalls != 1 {
		t.Fatalf("shadow-ban observation=%q calls=%d", got, shadowRepo.listCalls)
	}
}

type cancelAfterLastOnlineSuccess struct {
	*lastOnlineCommandQueriesStub
	cancel context.CancelFunc
}

func (s *cancelAfterLastOnlineSuccess) LastOnline(ctx context.Context, target string) (repository.LastOnlineRecord, error) {
	record, err := s.lastOnlineCommandQueriesStub.LastOnline(ctx, target)
	s.cancel()
	return record, err
}

func TestTextObservationAuditCancellationAfterSourceSuccessKeepsTextWithoutDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	queries := &cancelAfterLastOnlineSuccess{
		lastOnlineCommandQueriesStub: &lastOnlineCommandQueriesStub{record: repository.LastOnlineRecord{
			Found:          true,
			LastMessage:    sql.NullString{String: "completed source fact", Valid: true},
			LastSeenMillis: sql.NullInt64{Int64: 0, Valid: true},
		}},
		cancel: cancel,
	}
	underlying := &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Queries: queries}}}
	capture := &agentCaptureEngine{Engine: underlying}
	definition, _ := commandDefinitionFor("lastonline")
	status, err := definition.New(capture, &model.ChatMessage{Name: "caller", Text: "!lastonline target"}).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, context.Canceled) || len(underlying.chats) != 0 || !capture.dataObserved {
		t.Fatalf("status=%s err=%v chats=%v observed=%v data=%s", status, err, underlying.chats, capture.dataObserved, capture.data)
	}
	var observation commandTextObservation
	if json.Unmarshal(capture.data, &observation) != nil || !strings.Contains(observation.Text, "completed source fact") || queries.calls != 1 {
		t.Fatalf("canceled source observation=%s", capture.data)
	}
}
