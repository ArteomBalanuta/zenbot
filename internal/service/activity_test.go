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
	want := "\\n```Text\\n\n\n+----------+----------------+--------+--------------------------+\n|   TRIP   |  DAY_OF_WEEK   |  HOUR  |  PROBABILITY_PERCENTAGE  |\n+----------+----------------+--------+--------------------------+\n|  Trip-A  |     Sunday     |   1    |           50.0           |\n+----------+----------------+--------+--------------------------+\n\n\\n ```"
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

func TestActivityServiceReturnsRepositoryErrorTextAsSourceResult(t *testing.T) {
	want := "sentinel activity database failure"
	got, err := (&ActivityService{Repo: &activityRepositoryStub{err: errors.New(want)}}).Stats(context.Background(), "trip")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != want {
		t.Fatalf("payload=%q, want %q", got, want)
	}
}
