package service

import (
	"zenbot/bot/model"
)

type SecurityService struct {
	WhitelistedTrips []string
}

func NewSecurityService() *SecurityService {
	var trips []string = []string{"595754"}
	return &SecurityService{
		WhitelistedTrips: trips,
	}
}

func (s *SecurityService) AuthorizeUser(u *model.User) {
	s.WhitelistedTrips = append(s.WhitelistedTrips, u.Trip)
}

func (s *SecurityService) AuthorizeTrip(trip string) {
	s.WhitelistedTrips = append(s.WhitelistedTrips, trip)
}

func (s *SecurityService) IsAuthorized(u *model.User) bool {
	for _, v := range s.WhitelistedTrips {
		if v == u.Trip {
			return true
		}
	}
	return false
}
