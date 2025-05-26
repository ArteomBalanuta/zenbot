package service

import (
	"log"
	"zenbot/bot/config"
	"zenbot/bot/model"
)

type SecurityService struct {
	AdminTrips []string
}

func NewSecurityService(c *config.Config) *SecurityService {
	return &SecurityService{
		AdminTrips: c.AdminTrips,
	}
}

func (s *SecurityService) AuthorizeUser(u *model.User) {
	s.AdminTrips = append(s.AdminTrips, u.Trip)
}

func (s *SecurityService) AuthorizeTrip(trip string) {
	s.AdminTrips = append(s.AdminTrips, trip)
}

func (s *SecurityService) IsAuthorized(u *model.User, r *model.Role) bool {
	for _, v := range s.AdminTrips {
		if v == u.Trip {
			return true
		}
	}

	log.Printf("Command role: %d, text: %s", int(*r), r.String())

	/* gonna have a place with to store/check against users permission */
	ur := model.TRUSTED
	return int(ur) <= int(*r)
}
