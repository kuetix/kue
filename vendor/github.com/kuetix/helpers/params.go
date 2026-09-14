package helpers

func GetFunctionOptions(key string, defaultValue interface{}, options ...map[string]interface{}) (result interface{}) {
	for _, group := range options {
		for k, v := range group {
			if k == key {
				return v
			}
		}
	}

	return defaultValue
}
