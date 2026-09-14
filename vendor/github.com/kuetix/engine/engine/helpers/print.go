package helpers

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/kuetix/engine/engine/domain/issues"
)

func PrintFirstLevelString(v any) string {
	var buf bytes.Buffer

	rv := reflect.ValueOf(v)

	switch rv.Kind() {
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			val := rv.MapIndex(key)

			switch val.Kind() {
			case reflect.Map:
				_, _ = fmt.Fprintf(&buf, "%v: <map>\n", key)

			case reflect.Slice, reflect.Array:
				_, _ = fmt.Fprintf(&buf, "%v: <slice>\n", key)

			default:
				_, _ = fmt.Fprintf(&buf, "%v: %v\n", key, val.Interface())
			}
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			val := rv.Index(i)

			switch val.Kind() {
			case reflect.Map:
				_, _ = fmt.Fprintf(&buf, "[%d]: <map>\n", i)

			case reflect.Slice, reflect.Array:
				_, _ = fmt.Fprintf(&buf, "[%d]: <slice>\n", i)

			default:
				_, _ = fmt.Fprintf(&buf, "[%d]: %v\n", i, val.Interface())
			}
		}
	default:
		switch rv.Type().String() {
		case "string":
			_, _ = fmt.Fprintf(&buf, "%q\n", v)
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"float32", "float64", "bool":
			_, _ = fmt.Fprintf(&buf, "%v\n", v)
		case "*issues.Issue":
			_, _ = fmt.Fprintf(&buf, "%+v\n", v.(*issues.Issue).Errors)
		}
	}

	return buf.String()
}
