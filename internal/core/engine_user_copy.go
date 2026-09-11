package core

import "zenbot/internal/model"

// copyRoomUser detaches the record and the JSON containers decoded into Flair.
func copyRoomUser(user *model.User) *model.User {
	copy := *user
	copy.Flair = copyFlairJSON(user.Flair)
	return &copy
}

// copyFlairJSON follows the value shapes produced by model.GetUser/GetUsers:
// objects and arrays are mutable; JSON scalar values can be retained directly.
func copyFlairJSON(flair interface{}) interface{} {
	switch value := flair.(type) {
	case map[string]interface{}:
		if value == nil {
			return value
		}
		copy := make(map[string]interface{}, len(value))
		for key, nested := range value {
			copy[key] = copyFlairJSON(nested)
		}
		return copy
	case []interface{}:
		if value == nil {
			return value
		}
		copy := make([]interface{}, len(value))
		for i, nested := range value {
			copy[i] = copyFlairJSON(nested)
		}
		return copy
	default:
		return value
	}
}
