package helpers

import (
	"reflect"
)

//goland:noinspection GoUnusedExportedFunction
func IsStruct(v any) bool {
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	return rv.IsValid() && rv.Kind() == reflect.Struct
}
