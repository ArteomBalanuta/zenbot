package model

type Role int

const (
	ADMIN Role = iota
	MODERATOR
	TRUSTED
	USER
	REGULAR
	PEST
)

var roleName = map[Role]string{
	ADMIN:     "Admin",
	MODERATOR: "Moderator",
	TRUSTED:   "Trusted",
	USER:      "User",
	REGULAR:   "Regular",
	PEST:      "Pest",
}

func (role *Role) String() string {
	return roleName[*role]
}
