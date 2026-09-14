package helpers

import (
	"reflect"
)

// Len returns the length of a slice, array, map, or string
//
//goland:noinspection GoUnusedExportedFunction
func Len(v interface{}) int {
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return val.Len()
	default:
		return -1
	}
}

// IsSlice returns true if the given interface is a slice
//
//goland:noinspection GoUnusedExportedFunction
func IsSlice(t interface{}) bool {
	switch reflect.TypeOf(t).Kind() {
	case reflect.Slice:
		return true
	default:
		return false
	}
}

// AppendStringUnique appends a string to a slice if it is not already in the slice
//
//goland:noinspection GoUnusedExportedFunction
func AppendStringUnique(slice []string, value string) []string {
	for _, v := range slice {
		if v == value {
			return slice // already in slice → do nothing
		}
	}
	return append(slice, value)
}

// AppendUnique appends all unique values from b to a
func AppendUnique(a, b []string) []string {
	seen := make(map[string]struct{}, len(a))

	// mark all from a
	for _, v := range a {
		seen[v] = struct{}{}
	}

	// append from b only if new
	for _, v := range b {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			a = append(a, v)
		}
	}

	return a
}
