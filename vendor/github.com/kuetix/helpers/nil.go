package helpers

import "reflect"

func IsNil(value interface{}) bool {
	// If the value is not valid, it cannot be nil
	if value == nil {
		return true
	}

	// Use reflection to check for nil
	val := reflect.ValueOf(value)
	switch val.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Slice:
		return val.IsNil()
	default:
		return false
	}
}
