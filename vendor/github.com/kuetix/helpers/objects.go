package helpers

import (
	"reflect"

	"github.com/mitchellh/mapstructure"
)

// FieldValue returns the value of a field in a struct or map
//
//goland:noinspection GoUnusedExportedFunction
func FieldValue(obj interface{}, field string) (interface{}, bool) {
	val := reflect.ValueOf(obj)

	// Ensure we're dealing with a struct or a pointer to a struct
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct && val.Kind() != reflect.Map {
		return nil, false
	}

	if val.Kind() != reflect.Map {
		// Get the ID field
		idField := val.FieldByName(field)
		if !idField.IsValid() {
			return nil, false
		}

		// Return the ID field value and a boolean indicating success
		return idField.Interface(), true
	} else {
		// Get the ID field
		keys := val.MapKeys()
		for _, key := range keys {
			if key.Kind() == reflect.String {
				if key.String() == field {
					if !key.IsValid() {
						return nil, false
					}
					return val.MapIndex(key).Interface(), true
				}
			}
		}

		// Return the ID field value and a boolean indicating success
		return nil, false
	}
}

// SetFieldValueString sets the value of a field in a struct or map
//
//goland:noinspection GoUnusedExportedFunction
func SetFieldValueString(obj interface{}, field string, value string) (interface{}, bool) {
	val := reflect.ValueOf(obj)

	// Ensure we're dealing with a struct or a pointer to a struct
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct && val.Kind() != reflect.Map {
		return nil, false
	}

	if val.Kind() != reflect.Map {
		// Get the ID field
		idField := val.FieldByName(field)
		if !idField.IsValid() {
			return nil, false
		}

		idField.SetString(value)

		// Return the ID field value and a boolean indicating success
		return idField.Interface(), true
	} else {
		// Get the ID field
		for _, key := range val.MapKeys() {
			if key.Kind() == reflect.String {
				if key.String() == field {
					if !key.IsValid() {
						return nil, false
					}
					val.MapIndex(key).SetString(value)
					return val.MapIndex(key).Interface(), true
				}
			}
		}

		// Return the ID field value and a boolean indicating success
		return nil, false
	}
}

// SetFieldValueInt sets the value of a field in a struct or map
//
//goland:noinspection GoUnusedExportedFunction
func SetFieldValueInt(obj interface{}, field string, value int) (interface{}, bool) {
	val := reflect.ValueOf(obj)

	// Ensure we're dealing with a struct or a pointer to a struct
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct && val.Kind() != reflect.Map {
		return nil, false
	}

	if val.Kind() != reflect.Map {
		// Get the ID field
		idField := val.FieldByName(field)
		if !idField.IsValid() {
			return nil, false
		}

		idField.SetInt(int64(value))

		// Return the ID field value and a boolean indicating success
		return idField.Interface(), true
	} else {
		// Get the ID field
		for _, key := range val.MapKeys() {
			if key.Kind() == reflect.String {
				if key.String() == field {
					if !key.IsValid() {
						return nil, false
					}
					val.MapIndex(key).SetInt(int64(value))
					return val.MapIndex(key).Interface(), true
				}
			}
		}

		// Return the ID field value and a boolean indicating success
		return nil, false
	}
}

// SetFieldValue sets the value of a field in a struct or map
//
//goland:noinspection GoUnusedExportedFunction
func SetFieldValue(obj interface{}, field string, value any) (interface{}, bool) {
	val := reflect.ValueOf(obj)

	// Ensure we're dealing with a struct or a pointer to a struct
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct && val.Kind() != reflect.Map {
		return nil, false
	}

	if val.Kind() != reflect.Map {
		// Get the ID field
		idField := val.FieldByName(field)
		if !idField.IsValid() {
			return nil, false
		}

		idField.Set(reflect.ValueOf(value))

		// Return the ID field value and a boolean indicating success
		return idField.Interface(), true
	} else {
		// Get the ID field
		for _, key := range val.MapKeys() {
			if key.Kind() == reflect.String {
				if key.String() == field {
					if !key.IsValid() {
						return nil, false
					}
					val.MapIndex(key).Set(reflect.ValueOf(value))
					return val.MapIndex(key).Interface(), true
				}
			}
		}

		// Return the ID field value and a boolean indicating success
		return nil, false
	}
}

// CloneOf returns a deep copy of the given entity
//
//goland:noinspection GoUnusedExportedFunction
func CloneOf(entity interface{}) (interface{}, bool) {

	// Define a variable to hold the type of the struct
	var structType reflect.Type

	// Assign the type of the struct to the variable
	structType = reflect.TypeOf(entity)
	value := reflect.ValueOf(entity)

	var cloneOf interface{}
	isPtr := false
	if structType.Kind() == reflect.Ptr {
		isPtr = true
		cloneOf = reflect.New(structType.Elem()).Elem().Interface()

		value = value.Elem()
	} else {
		cloneOf = reflect.New(structType).Interface()
	}

	// var err error
	i := value.Interface()
	err := mapstructure.Decode(i, &cloneOf)
	if err != nil {
		return nil, false
	}

	return cloneOf, isPtr
}
