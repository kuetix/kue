package helpers

import (
	"fmt"
	"strconv"
)

func MustInt(i interface{}, byDefault ...int) (intValue int, isType string) {
	if i == nil {
		if len(byDefault) > 0 {
			return byDefault[0], "nil"
		}
		return 0, "nil"
	}

	switch i.(type) {
	case complex64:
		return int(real(i.(complex64))), "complex64"
	case complex128:
		return int(real(i.(complex128))), "complex128"
	case float32:
		return int(i.(float32)), "float32"
	case float64:
		return int(i.(float64)), "float64"
	case int64:
		return int(i.(int64)), "int64"
	case int32:
		return int(i.(int32)), "int32"
	case int16:
		return int(i.(int16)), "int16"
	case int8:
		return int(i.(int8)), "int8"
	case int:
		return i.(int), "int"
	case uint64:
		return int(i.(uint64)), "uint64"
	case uint32:
		return int(i.(uint32)), "uint32"
	case uint16:
		return int(i.(uint16)), "uint16"
	case uint8:
		return int(i.(uint8)), "uint8"
	case uint:
		return int(i.(uint)), "uint"
	case bool:
		if i.(bool) {
			return 1, "bool"
		}
		return 0, "bool"
	case nil:
		return 0, "nil"
	case string:
		si, err := strconv.Atoi(i.(string))
		if err == nil {
			return si, "string"
		}
	}

	if len(byDefault) > 0 {
		return byDefault[0], "default"
	}

	return 0, ""
}

func MustString(i interface{}, byDefault ...string) (stringValue string, isType string) {
	switch i.(type) {
	case complex64:
		return fmt.Sprintf("%v", i.(complex64)), "complex64"
	case complex128:
		return fmt.Sprintf("%v", i.(complex128)), "complex128"
	case float32:
		return fmt.Sprintf("%f", i.(float32)), "float32"
	case float64:
		return fmt.Sprintf("%f", i.(float64)), "float64"
	case int64:
		return fmt.Sprintf("%d", i.(int64)), "int64"
	case int32:
		return fmt.Sprintf("%d", i.(int32)), "int32"
	case int16:
		return fmt.Sprintf("%d", i.(int16)), "int16"
	case int8:
		return fmt.Sprintf("%d", i.(int8)), "int8"
	case int:
		return fmt.Sprintf("%d", i.(int)), "int"
	case uint64:
		return fmt.Sprintf("%d", i.(uint64)), "uint64"
	case uint32:
		return fmt.Sprintf("%d", i.(uint32)), "uint32"
	case uint16:
		return fmt.Sprintf("%d", i.(uint16)), "uint16"
	case uint8:
		return fmt.Sprintf("%d", i.(uint8)), "uint8"
	case uint:
		return fmt.Sprintf("%d", i.(uint)), "uint"
	case error:
		return i.(error).Error(), "error"
	case bool:
		if i.(bool) == true {
			return "true", "bool"
		}
		return "false", "bool"
	case nil:
		return "", "nil"
	case string:
		return i.(string), "string"
	}

	if len(byDefault) > 0 {
		return byDefault[0], "default"
	}

	return "", ""
}

func MustBool(i interface{}, byDefault ...bool) (boolValue bool, isType string) {
	switch i.(type) {
	case complex64:
		return true, "complex64"
	case complex128:
		return true, "complex128"
	case float32:
		return true, "float32"
	case float64:
		return true, "float64"
	case int64:
		return true, "int64"
	case int32:
		return true, "int32"
	case int16:
		return true, "int16"
	case int8:
		return true, "int8"
	case int:
		return true, "int"
	case uint64:
		return true, "uint64"
	case uint32:
		return true, "uint32"
	case uint16:
		return true, "uint16"
	case uint8:
		return true, "uint8"
	case uint:
		return true, "uint"
	case error:
		return true, "error"
	case bool:
		return i.(bool), "bool"
	case nil:
		return false, "nil"
	case string:
		return true, "string"
	}

	if len(byDefault) > 0 {
		return byDefault[0], "default"
	}

	return false, ""
}

func MustArray(i interface{}, byDefault ...[]interface{}) (arrayValue []interface{}, isType string) {
	switch i.(type) {
	case string:
		str := i.(string)
		result := make([]interface{}, 1)
		result[0] = str
		return result, "array"
	case []string:
		strs := i.([]string)
		result := make([]interface{}, len(strs))
		for i, str := range strs {
			result[i] = str
		}
		return result, "array"
	case []interface{}:
		return i.([]interface{}), "array"
	}
	return byDefault[0], "default"
}
