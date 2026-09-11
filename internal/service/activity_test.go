package service

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/repository"
)

type activityRepositoryStub struct {
	stats  []repository.ActivityStat
	err    error
	target string
	calls  int
}

func (s *activityRepositoryStub) ActivityStats(_ context.Context, target string) ([]repository.ActivityStat, error) {
	s.calls++
	s.target = target
	return s.stats, s.err
}

func TestActivityServiceRendersSaturnTablePayload(t *testing.T) {
	repo := &activityRepositoryStub{stats: []repository.ActivityStat{{Trip: "Trip-A", DayOfWeek: "Sunday", Hour: "1", ProbabilityPercentage: "50.0"}}}
	got, err := (&ActivityService{Repo: repo}).Stats(context.Background(), "Trip-A")
	if err != nil {
		t.Fatal(err)
	}
	want := "\n```Text\n\n\n+----------+----------------+--------+--------------------------+\n|   TRIP   |  DAY_OF_WEEK   |  HOUR  |  PROBABILITY_PERCENTAGE  |\n+----------+----------------+--------+--------------------------+\n|  Trip-A  |     Sunday     |   1    |           50.0           |\n+----------+----------------+--------+--------------------------+\n\n\n ```"
	if got != want {
		t.Fatalf("payload=%q, want %q", got, want)
	}
	if repo.calls != 1 || repo.target != "Trip-A" {
		t.Fatalf("calls=%d target=%q", repo.calls, repo.target)
	}
}

func TestActivityServiceReturnsSourceNoActivityText(t *testing.T) {
	got, err := (&ActivityService{Repo: &activityRepositoryStub{}}).Stats(context.Background(), "absent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "No activity found." {
		t.Fatalf("payload=%q", got)
	}
}

func TestActivityServiceReturnsRepositoryErrorWithoutDiagnosticPayload(t *testing.T) {
	want := errors.New("sentinel activity database failure")
	got, err := (&ActivityService{Repo: &activityRepositoryStub{err: want}}).Stats(context.Background(), "trip")
	if !errors.Is(err, want) || got != "" {
		t.Fatalf("payload=%q err=%v", got, err)
	}
}

func TestActivityServiceRejectsMissingRepository(t *testing.T) {
	for _, service := range []*ActivityService{nil, {}} {
		got, err := service.Stats(context.Background(), "trip")
		if err == nil || got != "" {
			t.Fatalf("service=%#v payload=%q err=%v", service, got, err)
		}
	}
}

type cancelingActivityRepository struct{ cancel context.CancelFunc }

func (r cancelingActivityRepository) ActivityStats(context.Context, string) ([]repository.ActivityStat, error) {
	r.cancel()
	return []repository.ActivityStat{{Trip: "trip"}}, nil
}

func TestActivityServiceObservesCancellationBeforeAndAfterRepositoryRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &activityRepositoryStub{}
	if got, err := (&ActivityService{Repo: repo}).Stats(ctx, "trip"); !errors.Is(err, context.Canceled) || got != "" || repo.calls != 0 {
		t.Fatalf("pre-canceled payload=%q err=%v calls=%d", got, err, repo.calls)
	}

	ctx, cancel = context.WithCancel(context.Background())
	if got, err := (&ActivityService{Repo: cancelingActivityRepository{cancel: cancel}}).Stats(ctx, "trip"); !errors.Is(err, context.Canceled) || got != "" {
		t.Fatalf("mid-read cancellation payload=%q err=%v", got, err)
	}
}
