package service_test

import (
	"context"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/service"
)

func TestSecurityIdentityAuditConfiguredCredentialsAreExact(t *testing.T) {
	security := &service.SecurityService{AdminTrips: []string{"AbC123"}, UserTrips: []string{"UsEr12"}}
	for _, tc := range []struct {
		name  string
		check func() bool
		want  bool
	}{
		{"exact administrator", func() bool {
			allowed, err := security.IsAuthorizedContext(context.Background(), &model.User{Trip: "AbC123"}, model.ADMIN)
			if err != nil {
				t.Fatal(err)
			}
			return allowed
		}, true},
		{"different administrator credential", func() bool {
			allowed, err := security.IsAuthorizedContext(context.Background(), &model.User{Trip: "abc123"}, model.ADMIN)
			if err != nil {
				t.Fatal(err)
			}
			return allowed
		}, false},
		{"exact user whitelist", func() bool { return security.IsConfiguredUserTrip("UsEr12") }, true},
		{"different user whitelist credential", func() bool { return security.IsConfiguredUserTrip("user12") }, false},
		{"exact lifecycle whitelist", func() bool { return security.IsLifecycleAuthorized(&model.User{Trip: "UsEr12"}) }, true},
		{"different lifecycle credential", func() bool { return security.IsLifecycleAuthorized(&model.User{Trip: "user12"}) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.check(); got != tc.want {
				t.Fatalf("authorized=%t; want %t", got, tc.want)
			}
		})
	}
}

func TestSecurityIdentityAuditMissingRepositoryDoesNotElevateUnknownUser(t *testing.T) {
	security := &service.SecurityService{}
	for _, required := range []model.Role{model.TRUSTED, model.USER} {
		allowed, err := security.IsAuthorizedContext(context.Background(), &model.User{Trip: "unknown"}, required)
		if err != nil {
			t.Fatal(err)
		}
		if allowed {
			t.Fatalf("unknown user received role %s without a repository or configured grant", required.String())
		}
	}
}
