package helpers

import "reflect"

//goland:noinspection GoUnusedExportedFunction
func IsPointer(v interface{}) bool {
	return reflect.TypeOf(v).Kind() == reflect.Ptr
}
