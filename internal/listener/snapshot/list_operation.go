package snapshot

import (
	"sort"
	"strings"

	"zenbot/internal/model"
)

// ListRoomOperation renders a source-compatible, stable remote room listing.
type ListRoomOperation struct{}

func NewListRoomOperation() ListRoomOperation { return ListRoomOperation{} }

func (ListRoomOperation) Apply(_ RoomSnapshotContext, snapshot Snapshot) (OperationResult, error) {
	return Success(FormatUsers(snapshot.Users)), nil
}

// FormatUsers deduplicates identities and sorts by hash before rendering.
func FormatUsers(users []*model.User) string {
	unique := make(map[string]*model.User, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		key := model.IdentityKey(user.Trip, user.Hash, user.Name)
		if _, exists := unique[key]; !exists {
			copy := *user
			unique[key] = &copy
		}
	}
	ordered := make([]*model.User, 0, len(unique))
	for _, user := range unique {
		ordered = append(ordered, user)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Hash == ordered[j].Hash {
			return strings.ToLower(ordered[i].Name) < strings.ToLower(ordered[j].Name)
		}
		return ordered[i].Hash < ordered[j].Hash
	})

	var output strings.Builder
	output.WriteString(`\nUsers online: \n`)
	for _, user := range ordered {
		trip := user.Trip
		if trip == "" {
			trip = "------"
		}
		output.WriteString(user.Hash)
		output.WriteString(" - ")
		output.WriteString(trip)
		output.WriteString(" - ")
		output.WriteString(user.Name)
		output.WriteString(`\n`)
	}
	output.WriteString(`\n`)
	return output.String()
}
