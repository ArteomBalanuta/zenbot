package service

import (
	"context"
	"log"
	"strings"
	"zenbot/internal/config"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type SecurityService struct {
	AdminTrips    []string
	UserTrips     []string
	Authorization repository.AuthorizationRepository
}

// The variadic repository preserves the original constructor for callers that
// only need configured-trip authorization, while production injects H2.
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
	trip = strings.TrimSpace(trip)
	if trip == "" {
		return nil
	}
	if s.Authorization != nil {
		if err := s.Authorization.GrantTrip(context.Background(), trip, model.ADMIN); err != nil {
			log.Printf("authorize trip %q: %v", trip, err)
			return err
		}
	}
	for _, existing := range s.AdminTrips {
		if existing == trip {
			return nil
		}
	}
	s.AdminTrips = append(s.AdminTrips, trip)
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
		if strings.EqualFold(strings.TrimSpace(trip), "x") || strings.EqualFold(strings.TrimSpace(trip), strings.TrimSpace(u.Trip)) {
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
		if strings.EqualFold(configured, "x") || strings.EqualFold(configured, trip) {
			return true
		}
	}
	return false
}

func (s *SecurityService) IsAuthorizedContext(ctx context.Context, u *model.User, required model.Role) (bool, error) {
	if u == nil {
		return false, nil
	}
	if s.Authorization != nil {
		return s.Authorization.IsTripAuthorized(ctx, u.Trip, required, s.AdminTrips)
	}
	for _, trip := range s.AdminTrips {
		if strings.EqualFold(strings.TrimSpace(trip), "x") || strings.EqualFold(strings.TrimSpace(trip), strings.TrimSpace(u.Trip)) {
			return true, nil
		}
	}
	// Zenbot's established ordering is strongest (ADMIN) first; lower numeric
	// roles therefore satisfy a command requiring a higher numeric threshold.
	return model.TRUSTED <= required, nil
}
