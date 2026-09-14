package helpers

import (
	"reflect"
	"unsafe"

	"github.com/kuetix/logger"
)

// CalculateMemoryUsage estimates the memory usage of an interface{} and all its nested elements
func CalculateMemoryUsage(v interface{}, visited map[uintptr]bool) uintptr {
	// Use a variable to accumulate the total size
	var size uintptr

	defer func(size *uintptr) {
		if r := recover(); r != nil {
			// If there was a panic, return zero sizes
			logger.Debug("Recovered from panic:", r)
		}
	}(&size)

	// Use reflection to get the underlying type
	value := reflect.ValueOf(v)

	// If it's a nil value, return zero sizes
	if !value.IsValid() {
		return 0
	}

	switch value.Kind() {
	case reflect.Ptr, reflect.Interface:
		// If it's a pointer or interface, check if we've already visited this memory address
		ptr := value.Pointer()
		if ptr != 0 {
			if visited[ptr] {
				return 0 // Skip already visited pointers to avoid infinite recursion
			}
			visited[ptr] = true
			size += unsafe.Sizeof(ptr) // Add the size of the pointer itself
			// Recursively calculate the size of the pointed-to value
			size += CalculateMemoryUsage(value.Elem().Interface(), visited)
		}
	case reflect.Struct:
		// For structs, add the size of each field recursively
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			// Check if the field is exported
			if field.CanInterface() {
				size += CalculateMemoryUsage(field.Interface(), visited)
			} else {
				// Use `unsafe.Sizeof` for unexported fields since we can't access their interface
				size += field.Type().Size()
			}
		}
	case reflect.Slice, reflect.Array:
		// For slices and arrays, add the size of each element recursively
		for i := 0; i < value.Len(); i++ {
			size += CalculateMemoryUsage(value.Index(i).Interface(), visited)
		}
	case reflect.Map:
		// For maps, add the size of each key and value recursively
		for _, key := range value.MapKeys() {
			size += CalculateMemoryUsage(key.Interface(), visited)
			size += CalculateMemoryUsage(value.MapIndex(key).Interface(), visited)
		}
	case reflect.String:
		// For strings, add the size of the string header and the string's contents
		size += unsafe.Sizeof("") + uintptr(value.Len())
	default:
		// For other types (e.g., int, float, bool), get the size directly
		size += value.Type().Size()
	}

	return size
}
