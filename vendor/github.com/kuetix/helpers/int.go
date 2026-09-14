package helpers

import (
	"strconv"
)

// IsBytesIsInt checks if a byte slice is a valid integer
//
//goland:noinspection GoUnusedExportedFunction
func IsBytesIsInt(value []byte) (intValue int, isInt bool) {
	// Attempt to convert to integer
	if intValue, err := strconv.Atoi(string(value)); err == nil {
		return intValue, true
	}

	return 0, false
}

//goland:noinspection GoUnusedExportedFunction
func IsNumeric(i interface{}) bool {
	switch i.(type) {
	case uint8:
		return true
	case uint16:
		return true
	case uint32:
		return true
	case uint64:
		return true
	case uint:
		return true
	case complex64:
		return true
	case complex128:
		return true
	case float32:
		return true
	case float64:
		return true
	case int64:
		return true
	case int32:
		return true
	case int16:
		return true
	case int8:
		return true
	case int:
		return true
	}
	return false
}
