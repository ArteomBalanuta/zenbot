package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"zenbot/internal/config"
	"zenbot/internal/model"
	"zenbot/internal/service"
	"zenbot/internal/testutil/sqlitefixture"
)

func TestSecurityPersistedDemotionRevokesRuntimeGrant(t *testing.T) {
	d := sqlitefixture.Open(t, "security-demotion")
	s := service.NewSecurityService(&config.Config{}, d)
	if err := s.AuthorizeTrip("Trip"); err != nil {
		t.Fatal(err)
	}
	allowed, err := s.IsAuthorizedContext(context.Background(), &model.User{Trip: "Trip"}, model.ADMIN)
	if err != nil || !allowed {
		t.Fatalf("grant: allowed=%t err=%v", allowed, err)
	}
	if err := d.GrantTrip(context.Background(), "Trip", model.REGULAR); err != nil {
		t.Fatal(err)
	}
	allowed, err = s.IsAuthorizedContext(context.Background(), &model.User{Trip: "Trip"}, model.ADMIN)
	if err != nil || allowed {
		t.Fatalf("demotion: allowed=%t err=%v configured=%v", allowed, err, s.AdminTrips)
	}
}

func TestSecurityConcurrentRuntimeAuthorizationOwnsConfiguration(t *testing.T) {
	config := &config.Config{AdminTrips: []string{"Root"}, UserTrips: []string{"User"}}
	s := service.NewSecurityService(config)
	config.AdminTrips[0] = "changed"
	config.UserTrips[0] = "changed"
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			trip := fmt.Sprintf("Trip%d", i)
			if err := s.AuthorizeTrip(trip); err != nil {
				t.Error(err)
			}
			ok, err := s.IsAuthorizedContext(context.Background(), &model.User{Trip: trip}, model.ADMIN)
			if err != nil || !ok {
				t.Errorf("runtime grant missing: %t %v", ok, err)
			}
			if !s.IsConfiguredUserTrip("User") {
				t.Error("configuration aliased")
			}
		}(i)
	}
	wg.Wait()
	if len(s.AdminTrips) != 1 || s.AdminTrips[0] != "Root" {
		t.Fatalf("runtime grant changed configuration: %v", s.AdminTrips)
	}
}

func TestSecurityExactWildcardAndRegularFallbackControls(t *testing.T) {
	for _, tc := range []struct {
		configured, trip string
		want             bool
	}{
		{"x", "anything", true}, {"X", "anything", false}, {"X", "X", true}, {"", "", false},
	} {
		s := service.NewSecurityService(&config.Config{AdminTrips: []string{tc.configured}})
		ok, err := s.IsAuthorizedContext(context.Background(), &model.User{Trip: tc.trip}, model.ADMIN)
		if err != nil || ok != tc.want {
			t.Errorf("configured=%q trip=%q allowed=%t err=%v", tc.configured, tc.trip, ok, err)
		}
	}
	s := service.NewSecurityService(&config.Config{})
	for _, role := range []model.Role{model.ADMIN, model.MODERATOR, model.TRUSTED, model.USER, model.REGULAR, model.PEST} {
		ok, err := s.IsAuthorizedContext(context.Background(), &model.User{Trip: "unknown"}, role)
		if err != nil || ok != (role == model.REGULAR || role == model.PEST) {
			t.Errorf("role=%v allowed=%t err=%v", role, ok, err)
		}
	}
}
