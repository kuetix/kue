package helpers

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
)

// FromMap populates a struct from a map[string]interface{}
//
//goland:noinspection GoUnusedExportedFunction
func FromMap(c interface{}, record map[string]interface{}) error {
	err := mapstructure.Decode(record, &c)
	if err != nil {
		return err
	}

	return nil
}

// ToMap converts a struct to a map[string]interface{}
//
//goland:noinspection GoUnusedExportedFunction
func ToMap(c interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	err := mapstructure.Decode(c, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ToMapRecursive converts a struct to a map[string]interface{} recursively
//
//goland:noinspection GoUnusedExportedFunction
func ToMapRecursive(c interface{}) (map[string]interface{}, error) {
	if c == nil {
		return map[string]interface{}{}, nil
	}

	visited := make(map[string]bool)
	root := convertValue(c, visited)

	if result, ok := root.(map[string]interface{}); ok {
		return result, nil
	}

	// Keep backward-compatible return type for non-map roots.
	return map[string]interface{}{"value": root}, nil
}

// convertValue recursively converts a value to map[string]interface{} if it's a struct,
// or processes nested slices and maps
func convertValue(val interface{}, visited map[string]bool) interface{} {
	if val == nil {
		return val
	}

	rv := reflect.ValueOf(val)
	for rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	// Check if it's a channel - channels cannot be JSON marshalled
	if rv.Kind() == reflect.Chan {
		return fmt.Sprintf("channel:%x", rv.Pointer())
	}

	// Check if it's a function - functions cannot be JSON marshalled
	if rv.Kind() == reflect.Func {
		if rv.IsNil() {
			return fmt.Sprintf("func:%p", rv.Interface())
		}
		if !rv.IsValid() {
			return fmt.Sprintf("func:%p", rv.Interface())
		}
		if rv.IsValid() && rv.Type().String() == "func() error" {
			return fmt.Sprintf("func:%p", rv.Interface())
		}
		if rv.Type().String() == "func() (io.ReadCloser, error)" {
			rv = rv.Call([]reflect.Value{})[0]
			val = rv.Interface()
		} else {
			return fmt.Sprintf("func:%p", rv.Interface())
		}
	}

	// Handle pointers - check for circular references before dereferencing
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return fmt.Sprintf("ptr:nil")
		}
		// Track pointer address to prevent circular references
		ptrAddr := fmt.Sprintf("ptr:%x", rv.Pointer())
		if visited[ptrAddr] {
			return fmt.Sprintf("ptr:cycle")
		}
		visited[ptrAddr] = true
		rv = rv.Elem()
		val = rv.Interface()
	}

	switch rv.Kind() {
	case reflect.Map:
		if rv.IsNil() {
			return fmt.Sprintf("map:nil")
		}

		mapAddr := fmt.Sprintf("map:%x", rv.Pointer())
		if rv.Pointer() != 0 {
			if visited[mapAddr] {
				return fmt.Sprintf("map:cycle")
			}
			visited[mapAddr] = true
		}

		// Convert to map[string]interface{}
		mapVal := make(map[string]interface{})
		for _, key := range rv.MapKeys() {
			keyVal := rv.MapIndex(key).Interface()
			defer func() {
				if err := recover(); err != nil {
					keyVal = fmt.Sprintf("%v", err)
				}
			}()
			if keyVal == nil {
				keyVal = "nil"
			}
			if reflect.TypeOf(keyVal).Kind() == reflect.Slice && reflect.TypeOf(keyVal).Elem().Kind() == reflect.Uint8 {
				keyVal = fmt.Sprintf("%s %v", keyVal, keyVal)
			}
			mapVal[fmt.Sprintf("%v", key.Interface())] = convertValue(keyVal, visited)
		}
		return mapVal
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return fmt.Sprintf("slice:nil")
		}

		if rv.Kind() == reflect.Slice && rv.Pointer() != 0 {
			sliceAddr := fmt.Sprintf("slice:%x", rv.Pointer())
			if visited[sliceAddr] {
				return fmt.Sprintf("slice:cycle")
			}
			visited[sliceAddr] = true
		}

		// Process array elements
		result := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = convertValue(rv.Index(i).Interface(), visited)
		}
		return result
	case reflect.Struct:
		nested := make(map[string]interface{})
		t := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			fieldType := t.Field(i)
			if !fieldType.IsExported() {
				continue
			}

			fieldName := fieldType.Name
			if tag := fieldType.Tag.Get("json"); tag != "" {
				parts := strings.Split(tag, ",")
				if parts[0] == "-" {
					continue
				}
				if parts[0] != "" {
					fieldName = parts[0]
				}
			}

			nested[fieldName] = convertValue(rv.Field(i).Interface(), visited)
		}
		return nested
	case reflect.Complex64, reflect.Complex128, reflect.UnsafePointer:
		return fmt.Sprintf("%v", rv.Interface())
	default:
		// Primitive types and other kinds
		return rv.Interface()
	}
}

// IsPathExists checks if a nested path exists in a map[string]interface{}
//
//goland:noinspection GoUnusedExportedFunction
func IsPathExists(data map[string]interface{}, path []string) bool {
	current := data
	for i, key := range path {
		// Check if key exists at current level
		val, ok := current[key]
		if !ok {
			fmt.Printf("Path broken at %s (index %d)\n", key, i)
			return false
		}

		// If this is the last key in the path, it's found
		if i == len(path)-1 {
			return true
		}

		// Check if the value is a nested map
		nested, ok := val.(map[string]interface{})
		if !ok {
			fmt.Printf("Value at %s (index %d) is not a map\n", key, i)
			return false
		}
		current = nested
	}
	return true
}

// MapKey creates a nested map if it doesn't exist, and returns the nested map.
//
//goland:noinspection GoUnusedExportedFunction
func MapKey(m *map[string]interface{}, key string) map[string]interface{} {
	if _, ok := (*m)[key]; !ok {
		(*m)[key] = make(map[string]interface{})
	}

	return (*m)[key].(map[string]interface{})
}

// MapPtrKey creates a nested map if it doesn't exist, and returns the nested map.
//
//goland:noinspection GoUnusedExportedFunction
func MapPtrKey(m *map[string]interface{}, key string) *map[string]interface{} {
	if _, ok := (*m)[key]; !ok {
		(*m)[key] = make(map[string]interface{})
	}

	return (*m)[key].(*map[string]interface{})
}

// DecodeToMap decodes an interface{} into a map[string]interface{}
//
//goland:noinspection GoUnusedExportedFunction
func DecodeToMap(input interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := mapstructure.Decode(input, &result)
	return result, err
}

// MergeObjectsToMap merges multiple objects into a single map, excluding keys specified in the exclude map.
//
//goland:noinspection GoUnusedExportedFunction
func MergeObjectsToMap(exclude map[string]interface{}, objs ...interface{}) *map[string]interface{} {
	merged := make(map[string]interface{})
	for i, o := range objs {
		m, _ := DecodeToMap(o)
		if i == 0 {
			mergeMapsRecursively(merged, m, merged, map[string]interface{}{"IncludeEmpty": true})
			continue
		}
		mergeMapsRecursively(merged, m, exclude, map[string]interface{}{"IncludeEmpty": true})
	}
	return &merged
}
