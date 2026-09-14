package helpers

import "reflect"

func IsEmptyValue(value any) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)

	return IsEmptyReflectValue(v)
}

func IsEmptyReflectValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !IsEmptyReflectValue(v.Field(i)) {
				return false
			}
		}
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		// Channels, functions, and unsafe pointers are considered empty if nil
		if v.IsNil() {
			return true
		}
	default:
		// For other types, we consider them non-empty
		return false
	}
	return false
}
