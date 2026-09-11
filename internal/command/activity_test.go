package command

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type activityCommandRepositoryStub struct {
	stats  []repository.ActivityStat
	err    error
	target string
	calls  int
}

func (s *activityCommandRepositoryStub) ActivityStats(_ context.Context, target string) ([]repository.ActivityStat, error) {
	s.calls++
	s.target = target
	return s.stats, s.err
}

func TestActivityCommandPreservesAliasesRoleFirstArgumentAndWhisper(t *testing.T) {
	repo := &activityCommandRepositoryStub{stats: []repository.ActivityStat{{Trip: "trip", DayOfWeek: "Sunday", Hour: "1", ProbabilityPercentage: "100.0"}}}
	engine := &commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: repo}}}
	definition, ok := commandDefinitionFor("activity")
	if !ok || definition.Canonical != "active" || definition.Role != model.MODERATOR {
		t.Fatalf("definition=%+v ok=%v", definition, ok)
	}
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!activity  trip ignored", IsWhisper: true}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if repo.calls != 1 || repo.target != "trip" {
		t.Fatalf("calls=%d target=%q", repo.calls, repo.target)
	}
	if len(engine.chats) != 1 || engine.chats[0] != "alice|Stats: \n\n```Text\n\n\n+--------+----------------+--------+--------------------------+\n|  TRIP  |  DAY_OF_WEEK   |  HOUR  |  PROBABILITY_PERCENTAGE  |\n+--------+----------------+--------+--------------------------+\n|  trip  |     Sunday     |   1    |          100.0           |\n+--------+----------------+--------+--------------------------+\n\n\n ```|true" {
		t.Fatalf("chats=%v", engine.chats)
	}
}

func TestActivityCommandMissingTargetUsesSourceExampleWithoutQuery(t *testing.T) {
	repo := &activityCommandRepositoryStub{}
	engine := &commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: repo}}}
	definition, _ := commandDefinitionFor("active")
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!active"}).Execute(context.Background())
	if err != nil || status != model.FAILED {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if repo.calls != 0 || len(engine.chats) != 1 || engine.chats[0] != "alice|Example: !active 8Wotmg|false" {
		t.Fatalf("calls=%d chats=%v", repo.calls, engine.chats)
	}
}

func TestActivityCommandReturnsRepositoryErrorWithoutReply(t *testing.T) {
	want := errors.New("sentinel activity database failure")
	repo := &activityCommandRepositoryStub{err: want}
	engine := &commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: repo}}}
	definition, _ := commandDefinitionFor("active")
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!active  trip ignored", IsWhisper: true}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, want) {
		t.Fatalf("status=%v err=%v chats=%v", status, err, engine.chats)
	}
	if repo.calls != 1 || repo.target != "trip" {
		t.Fatalf("calls=%d target=%q", repo.calls, repo.target)
	}
	if len(engine.chats) != 0 {
		t.Fatalf("chats=%v", engine.chats)
	}
}

func TestActivityCommandReturnsMissingTargetDeliveryFailure(t *testing.T) {
	want := errors.New("usage delivery failed")
	repo := &activityCommandRepositoryStub{}
	engine := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: repo}}}, sendErr: want}
	definition, _ := commandDefinitionFor("active")
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!active"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, want) || repo.calls != 0 {
		t.Fatalf("status=%v err=%v calls=%d", status, err, repo.calls)
	}
}

type cancellingActivityCommandRepository struct{ cancel context.CancelFunc }

func (s *cancellingActivityCommandRepository) ActivityStats(_ context.Context, _ string) ([]repository.ActivityStat, error) {
	s.cancel()
	return []repository.ActivityStat{{Trip: "trip", DayOfWeek: "Sunday", Hour: "1", ProbabilityPercentage: "100"}}, nil
}

func TestActivityCommandDoesNotReplyWhenContextCancelsBeforeQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &activityCommandRepositoryStub{}
	engine := &commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: repo}}}
	definition, _ := commandDefinitionFor("active")
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!active trip"}).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, context.Canceled) || repo.calls != 0 || len(engine.chats) != 0 {
		t.Fatalf("status=%v err=%v calls=%d chats=%v", status, err, repo.calls, engine.chats)
	}
}

func TestActivityCommandDoesNotReplyWhenContextCancelsDuringQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	engine := &commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: &cancellingActivityCommandRepository{cancel: cancel}}}}
	definition, _ := commandDefinitionFor("active")
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!active trip"}).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, context.Canceled) || len(engine.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, engine.chats)
	}
}

func TestRegisterUserUtilitiesAddsActivityOnlyWithActivityService(t *testing.T) {
	without := &commandEngineStub{}
	if err := RegisterUserUtilities(without); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"active", "activity"} {
		if _, ok := (*without.GetEnabledCommands())[alias]; ok {
			t.Fatalf("registered without activity service: %q", alias)
		}
	}
	with := &commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: &activityCommandRepositoryStub{}}}}
	if err := RegisterUserUtilities(with); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"active", "activity"} {
		if _, ok := (*with.GetEnabledCommands())[alias]; !ok {
			t.Fatalf("missing activity alias: %q", alias)
		}
	}
}
