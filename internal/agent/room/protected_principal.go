package room

import "strings"

type ProtectedPrincipalPolicy struct {
	Creator        string
	Administrators []string
	Bot, Master    string
}

func (p ProtectedPrincipalPolicy) IsProtected(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	if n == strings.ToLower(p.Creator) || n == strings.ToLower(p.Bot) || n == strings.ToLower(p.Master) {
		return true
	}
	for _, a := range p.Administrators {
		if n == strings.ToLower(a) {
			return true
		}
	}
	return false
}
