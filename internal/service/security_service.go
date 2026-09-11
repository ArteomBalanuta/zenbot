package service

import (
	"context"
	"log"
	"strings"
	"sync"
	"zenbot/internal/config"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type SecurityService struct {
	// Configured overrides are immutable after construction. Runtime grants
	// belong to storage, or to runtimeAdmins when no repository is installed.
	AdminTrips    []string
	UserTrips     []string
	Authorization repository.AuthorizationRepository
	grantMu       sync.RWMutex
	runtimeAdmins map[string]struct{}
}

// The variadic repository preserves the original constructor for callers that
// only need configured-trip authorization, while production injects SQLite.
func NewSecurityService(c *config.Config, auth ...repository.AuthorizationRepository) *SecurityService {
	s := &SecurityService{AdminTrips: append([]string(nil), c.AdminTrips...), UserTrips: append([]string(nil), c.UserTrips...)}
	if len(auth) > 0 {
		s.Authorization = auth[0]
	}
	return s
}

func (s *SecurityService) AuthorizeUser(u *model.User) error {
	if u != nil {
		return s.AuthorizeTrip(u.Trip)
	}
	return nil
}
func (s *SecurityService) AuthorizeTrip(trip string) error {
	return s.AuthorizeTripContext(context.Background(), trip)
}

func (s *SecurityService) AuthorizeTripContext(ctx context.Context, trip string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	trip = strings.TrimSpace(trip)
	if trip == "" {
		return nil
	}
	if s.Authorization != nil {
		if err := s.Authorization.GrantTrip(ctx, trip, model.ADMIN); err != nil {
			log.Printf("authorize trip %q: %v", trip, err)
			return err
		}
		return nil
	}
	s.grantMu.Lock()
	defer s.grantMu.Unlock()
	if s.runtimeAdmins == nil {
		s.runtimeAdmins = make(map[string]struct{})
	}
	s.runtimeAdmins[trip] = struct{}{}
	return nil
}

func (s *SecurityService) IsAuthorized(u *model.User, r *model.Role) bool {
	if u == nil || r == nil {
		return false
	}
	ok, err := s.IsAuthorizedContext(context.Background(), u, *r)
	if err != nil {
		log.Printf("authorization lookup failed: %v", err)
		return false
	}
	return ok
}

// IsLifecycleAuthorized preserves Saturn's restart/shutdown user-trip bypass.
func (s *SecurityService) IsLifecycleAuthorized(u *model.User) bool {
	if u == nil {
		return false
	}
	for _, trip := range s.UserTrips {
		if strings.TrimSpace(trip) == "x" || (strings.TrimSpace(u.Trip) != "" && strings.TrimSpace(trip) == strings.TrimSpace(u.Trip)) {
			return true
		}
	}
	admin := model.ADMIN
	return s.IsAuthorized(u, &admin)
}

// IsConfiguredUserTrip reports whether trip is explicitly allowed by the
// source-compatible per-command userTrips whitelist.
func (s *SecurityService) IsConfiguredUserTrip(trip string) bool {
	trip = strings.TrimSpace(trip)
	if trip == "" {
		return false
	}
	for _, configured := range s.UserTrips {
		configured = strings.TrimSpace(configured)
		if configured == "x" || configured == trip {
			return true
		}
	}
	return false
}

func (s *SecurityService) IsAuthorizedContext(ctx context.Context, u *model.User, required model.Role) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if u == nil {
		return false, nil
	}
	if s.Authorization != nil {
		return s.Authorization.IsTripAuthorized(ctx, u.Trip, required, append([]string(nil), s.AdminTrips...))
	}
	for _, trip := range s.AdminTrips {
		if strings.TrimSpace(trip) == "x" || (strings.TrimSpace(u.Trip) != "" && strings.TrimSpace(trip) == strings.TrimSpace(u.Trip)) {
			return true, nil
		}
	}
	s.grantMu.RLock()
	_, granted := s.runtimeAdmins[strings.TrimSpace(u.Trip)]
	s.grantMu.RUnlock()
	if granted {
		return true, nil
	}
	// Zenbot's established ordering is strongest (ADMIN) first; lower numeric
	// roles therefore satisfy a command requiring a higher numeric threshold.
	return model.REGULAR <= required, nil
}
