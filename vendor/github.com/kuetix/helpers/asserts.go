package helpers

// AssertInteger performs a comparison between an integer value and a test integer based on the provided operator.
// Supported operators: gt, lt, eq, ne, ge, le
//
//goland:noinspection GoUnusedExportedFunction
func AssertInteger(op string, value int, test int) bool {
	switch op {
	case "gt":
		return value > test
	case "lt":
		return value < test
	case "eq":
		return value == test
	case "ne":
		return value != test
	case "ge":
		return value >= test
	case "le":
		return value <= test
	}

	return false
}

// AssertString performs a comparison between a string value and a test string based on the provided operator.
// Supported operators: eq, ne
//
//goland:noinspection GoUnusedExportedFunction
func AssertString(op string, value string, test string) bool {
	switch op {
	case "eq":
		return value == test
	case "ne":
		return value != test
	}

	return false
}

//goland:noinspection GoUnusedExportedFunction
func AssertSwitch(value string, test map[string]interface{}) string {
	if v, ok := test[value]; ok {
		return v.(string)
	}

	return ""
}
