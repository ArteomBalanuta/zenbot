package service

import (
	"context"
	"errors"
	"testing"
	"zenbot/internal/config"
	"zenbot/internal/model"
)

type authRepo struct {
	role  model.Role
	calls int
	err   error
}

func (r *authRepo) IsTripAuthorized(_ context.Context, _ string, required model.Role, configured []string) (bool, error) {
	r.calls++
	for _, v := range configured {
		if v == "x" {
			return true, nil
		}
	}
	return r.role <= required, nil
}
func (r *authRepo) GrantTrip(context.Context, string, model.Role) error     { return r.err }
func (r *authRepo) ResolveRole(context.Context, string) (model.Role, error) { return r.role, nil }

func TestSecurityUsesPersistedAuthorizationAndConfiguredWildcard(t *testing.T) {
	r := &authRepo{role: model.MODERATOR}
	s := NewSecurityService(&config.Config{AdminTrips: []string{"x"}}, r)
	required := model.USER
	if !s.IsAuthorized(&model.User{Trip: "persisted"}, &required) {
		t.Fatal("persisted role should satisfy required USER")
	}
	if r.calls != 1 {
		t.Fatalf("repository calls=%d, want 1", r.calls)
	}
	if !s.IsAuthorized(&model.User{Trip: "other"}, &[]model.Role{model.ADMIN}[0]) {
		t.Fatal("wildcard should authorize")
	}
}

func TestSecurityFailsClosedOnNilPrincipalAndRole(t *testing.T) {
	s := NewSecurityService(&config.Config{})
	if s.IsAuthorized(nil, &[]model.Role{model.USER}[0]) {
		t.Fatal("nil user authorized")
	}
	if s.IsAuthorized(&model.User{}, nil) {
		t.Fatal("nil role authorized")
	}
}

func TestSecurityServiceAuthorizeTripPropagatesRepositoryError(t *testing.T) {
	errWant := errors.New("database unavailable")
	s := NewSecurityService(&config.Config{}, &authRepo{err: errWant})
	if err := s.AuthorizeTrip(" trip "); !errors.Is(err, errWant) {
		t.Fatalf("err=%v, want %v", err, errWant)
	}
	if len(s.AdminTrips) != 0 {
		t.Fatalf("failed authorization mutated AdminTrips: %v", s.AdminTrips)
	}
}
