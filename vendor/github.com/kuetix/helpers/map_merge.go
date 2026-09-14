package helpers

// mergeMapsRecursively merges two maps recursively, excluding keys specified in the exclude map.
// options:
// - IncludeEmpty: bool (default: true) - Include empty values in the merged map
func mergeMapsRecursively(dst, src map[string]interface{}, exclude map[string]interface{}, options ...map[string]interface{}) {
	excluded := false
	isIncludeEmpty := GetFunctionOptions("IncludeEmpty", true, options...).(bool)
	for k, v := range src {
		excludeMap := exclude
		if isIncludeEmpty || !IsEmptyValue(v) {
			if existingValue, ok := dst[k]; ok {
				if exclude != nil {
					if excludeValue, ok := exclude[k]; ok {
						excluded = true
						if excludeMap, ok = excludeValue.(map[string]interface{}); ok {
							if excludeMap != nil {
								excluded = false
							}
						}
					}
				}

				if !excluded {
					if existingMap, ok := existingValue.(map[string]interface{}); ok {
						if newMap, ok := v.(map[string]interface{}); ok {
							mergeMapsRecursively(existingMap, newMap, excludeMap)
							continue
						}
					}
				}
			}

			excluded = false
			if excludeValue, ok := exclude[k]; ok {
				excluded = true
				if excludeMap, ok = excludeValue.(map[string]interface{}); ok {
					if excludeMap != nil {
						dst[k] = make(map[string]interface{})
						mergeMapsRecursively(dst[k].(map[string]interface{}), v.(map[string]interface{}), excludeMap, options...)
						excluded = false
						continue
					}
				}
			}

			if !excluded {
				dst[k] = v
			}
		}
	}
}

// MergeMapsLevel0 merges multiple maps into a single map without recursion.
//
//goland:noinspection GoUnusedExportedFunction
func MergeMapsLevel0(maps ...map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})
	for _, m := range maps {
		for k, v := range m {
			merged[k] = v
		}
	}

	return merged
}

// MergeMaps merges multiple maps into a single map.
//
//goland:noinspection GoUnusedExportedFunction
func MergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	for key, srcValue := range src {
		if dstValue, ok := dst[key]; ok {
			// If the key exists in both maps and both are maps, recurse
			switch dstValueTyped := dstValue.(type) {
			case map[string]interface{}:
				if srcValueTyped, ok := srcValue.(map[string]interface{}); ok {
					dst[key] = MergeMaps(dstValueTyped, srcValueTyped)
				} else {
					// If the source value isn't a map, overwrite the destination
					dst[key] = srcValue
				}
			case []string:
				if srcValueTyped, ok := srcValue.([]interface{}); ok {
					for _, v := range srcValueTyped {
						if val, ok := v.(string); ok {
							dst[key] = append(dstValueTyped, val)
						}
					}
				} else {
					// If the source value isn't a map, overwrite the destination
					if srcVTyped, ok := srcValue.(string); ok {
						dst[key] = append(dstValueTyped, srcVTyped)
					}
				}
			case []interface{}:
				if srcValueTyped, ok := srcValue.([]interface{}); ok {
					for _, v := range srcValueTyped {
						dst[key] = append(dstValueTyped, v)
					}
				} else {
					// If the source value isn't a map, overwrite the destination
					dst[key] = srcValue
				}
			default:
				// Overwrite the destination if it's not a map
				dst[key] = srcValue
			}
		} else {
			// Add a new key-value pair to the destination
			dst[key] = srcValue
		}
	}
	return dst
}

// UpdateMaps merges multiple maps into a single map.
// The first map is modified in place and returned as a pointer.
// If the first map is nil, a new map is created.
//
//goland:noinspection GoUnusedExportedFunction
func UpdateMaps(maps ...*map[string]interface{}) *map[string]interface{} {
	dst := maps[0]
	if dst == nil {
		dst = &map[string]interface{}{}
	}
	for i, src := range maps {
		if i == 0 {
			continue
		}
		UpdateMap(dst, *src)
	}

	return dst
}

// UpdateMap merges a map into another map.
// The first map is modified in place.
func UpdateMap(dst *map[string]interface{}, src map[string]interface{}) {
	for key, srcValue := range src {
		if dstValue, ok := (*dst)[key]; ok {
			// If the key exists in both maps and both are maps, recurse
			switch dstValueTyped := dstValue.(type) {
			case map[string]interface{}:
				if srcValueTyped, ok := srcValue.(map[string]interface{}); ok {
					(*dst)[key] = MergeMaps(dstValueTyped, srcValueTyped)
				} else {
					// If the source value isn't a map, overwrite the destination
					(*dst)[key] = srcValue
				}
			default:
				// Overwrite the destination if it's not a map
				(*dst)[key] = srcValue
			}
		} else {
			// Add a new key-value pair to the destination
			(*dst)[key] = srcValue
		}
	}
}
