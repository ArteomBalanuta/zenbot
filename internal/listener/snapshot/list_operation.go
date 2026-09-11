package snapshot

import (
	"encoding/json"
	"sort"
	"strings"

	"zenbot/internal/model"
)

// ListRoomOperation renders a source-compatible, stable remote room listing.
type ListRoomOperation struct{}

func NewListRoomOperation() ListRoomOperation { return ListRoomOperation{} }

func (ListRoomOperation) Apply(context RoomSnapshotContext, snapshot Snapshot) (OperationResult, error) {
	users := orderedUniqueUsers(snapshot.Users)
	names := make([]string, len(users))
	for index, user := range users {
		names[index] = user.Name
	}
	data, err := json.Marshal(struct {
		Room          string   `json:"room"`
		Users         []string `json:"users"`
		Count         int      `json:"count"`
		ReturnedCount int      `json:"returnedCount"`
		Truncated     bool     `json:"truncated"`
	}{
		Room:          context.TargetChannel,
		Users:         names,
		Count:         len(users),
		ReturnedCount: len(names),
		Truncated:     false,
	})
	if err != nil {
		return OperationResult{}, err
	}
	result := Success(formatUsers(users))
	result.Data = append(json.RawMessage(nil), data...)
	result.DataObserved = true
	return result, nil
}

// FormatUsers deduplicates identities and sorts by hash before rendering.
func FormatUsers(users []*model.User) string {
	return formatUsers(orderedUniqueUsers(users))
}

func orderedUniqueUsers(users []*model.User) []*model.User {
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
		if ordered[i].Hash != ordered[j].Hash {
			return ordered[i].Hash < ordered[j].Hash
		}
		leftName, rightName := strings.ToLower(ordered[i].Name), strings.ToLower(ordered[j].Name)
		if leftName != rightName {
			return leftName < rightName
		}
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		if ordered[i].Trip != ordered[j].Trip {
			return ordered[i].Trip < ordered[j].Trip
		}
		return model.IdentityKey(ordered[i].Trip, ordered[i].Hash, ordered[i].Name) < model.IdentityKey(ordered[j].Trip, ordered[j].Hash, ordered[j].Name)
	})
	return ordered
}

func formatUsers(ordered []*model.User) string {
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
