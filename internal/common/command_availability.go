package common

import "reflect"

// CommandAvailability lets a concrete owner narrow its static capability
// interfaces to dependencies actually installed at composition time.
type CommandAvailability interface {
	CommandAvailable(canonical string) bool
}

// DependencyConfigured reports structural presence, rejecting both nil
// interfaces and nil dynamic values. Admission and source selection agree here;
// it does not probe dependency health or turn operation errors into fallbacks.
func DependencyConfigured(value any) bool {
	if value == nil {
		return false
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !v.IsNil()
	}
	return true
}
