package workflow

import (
	"strconv"
	"strings"
)

// parseWSLValue parses a WSL inline value expression into a Go value.
// Supported:
// - array literals: [value1, value2, ...] (nested supported)
// - object literals: {key: value, ...} (nested supported)
// - numbers (int/float)
// - booleans: true/false
// - quoted strings: "text" or 'text'
// - placeholders: $Var or $Var.field -> converted to "<<Var[.field]>>" string for later resolution
// - bare identifiers -> left as string
func parseWSLValue(raw string) (interface{}, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	if strings.HasPrefix(raw, "[") {
		arr, ok := parseWSLArray(raw)
		if ok {
			return arr, true
		}
	}
	if strings.HasPrefix(raw, "{") {
		m, ok := parseWSLObject(raw)
		if ok {
			return m, true
		}
	}
	// number int
	if iv, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return int(iv), true
	}
	// number float
	if fv, err := strconv.ParseFloat(raw, 64); err == nil {
		return fv, true
	}
	// bool
	low := strings.ToLower(raw)
	if low == "true" {
		return true, true
	}
	if low == "false" {
		return false, true
	}
	// quoted string
	if len(raw) >= 2 && ((raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'')) {
		return strings.Trim(raw, "\"'"), true
	}
	// placeholder $Var or $Var.field
	if strings.HasPrefix(raw, "$") {
		suffix := strings.TrimPrefix(raw, "$")
		return "<<" + suffix + ">>", true
	}
	// bare identifier -> string
	return raw, true
}

// parseWSLObject parses a WSL object literal like {key: value, nested: { ... }}.
func parseWSLObject(raw string) (map[string]interface{}, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") {
		return nil, false
	}
	// remove outer braces if present
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	m := map[string]interface{}{}
	// scan for pairs key: value separated by commas, respecting nested braces and quotes
	i := 0
	n := len(raw)
	for i < n {
		// skip spaces and commas
		for i < n && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\t' || raw[i] == ',') {
			i++
		}
		if i >= n {
			break
		}
		// parse key (ident or quoted)
		keyStart := i
		inStr := byte(0)
		for i < n {
			c := raw[i]
			if inStr != 0 {
				if c == '\\' { // escape
					// Check bounds before advancing by 2
					if i+1 < n {
						i += 2
					} else {
						i++
					}
					continue
				}
				if c == inStr {
					i++
					break
				}
				i++
				continue
			}
			if c == '\'' || c == '"' {
				inStr = c
				i++
				continue
			}
			if c == ':' {
				break
			}
			i++
		}
		if i >= n || raw[i] != ':' {
			return nil, false
		}
		keyStr := strings.TrimSpace(raw[keyStart:i])
		// trim quotes from key if quoted
		if len(keyStr) >= 2 && ((keyStr[0] == '"' && keyStr[len(keyStr)-1] == '"') || (keyStr[0] == '\'' && keyStr[len(keyStr)-1] == '\'')) {
			keyStr = strings.Trim(keyStr, "\"'")
		}
		i++ // skip ':'
		// skip spaces
		for i < n && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\t') {
			i++
		}
		// parse value until top-level comma or end
		valStart := i
		brace := 0
		brack := 0
		paren := 0
		inStr = 0
		for i < n {
			c := raw[i]
			if inStr != 0 {
				if c == '\\' {
					// Check bounds before advancing by 2
					if i+1 < n {
						i += 2
					} else {
						i++
					}
					continue
				}
				if c == inStr {
					inStr = 0
					i++
					continue
				}
				i++
				continue
			}
			if c == '\'' || c == '"' {
				inStr = c
				i++
				continue
			}
			switch c {
			case '{':
				brace++
			case '}':
				if brace > 0 {
					brace--
				}
			case '[':
				brack++
			case ']':
				if brack > 0 {
					brack--
				}
			case '(':
				paren++
			case ')':
				if paren > 0 {
					paren--
				}
			case ',':
				if brace == 0 && brack == 0 && paren == 0 {
					goto haveValue
				}
			}
			i++
		}
	haveValue:
		valStr := strings.TrimSpace(raw[valStart:i])
		if v, ok := parseWSLValue(valStr); ok {
			m[keyStr] = v
		}
		// skip trailing spaces and optional comma
		for i < n && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\t' || raw[i] == ',') {
			i++
		}
	}
	return m, true
}

// parseWSLArray parses a WSL array literal like [value1, value2, ...].
// Supports nested arrays and objects: [[1, 2], {key: value}]
func parseWSLArray(raw string) ([]interface{}, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "[") {
		return nil, false
	}
	// remove outer brackets if present
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	// empty array
	if raw == "" {
		return []interface{}{}, true
	}
	arr := []interface{}{}
	// scan for elements separated by commas, respecting nested brackets, braces and quotes
	i := 0
	n := len(raw)
	for i < n {
		// skip spaces and commas
		for i < n && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\t' || raw[i] == ',') {
			i++
		}
		if i >= n {
			break
		}
		// parse value until top-level comma or end
		valStart := i
		brace := 0
		brack := 0
		paren := 0
		inStr := byte(0)
		for i < n {
			c := raw[i]
			if inStr != 0 {
				if c == '\\' {
					// Check bounds before advancing by 2
					if i+1 < n {
						i += 2
					} else {
						i++
					}
					continue
				}
				if c == inStr {
					inStr = 0
					i++
					continue
				}
				i++
				continue
			}
			if c == '\'' || c == '"' {
				inStr = c
				i++
				continue
			}
			switch c {
			case '{':
				brace++
			case '}':
				if brace > 0 {
					brace--
				}
			case '[':
				brack++
			case ']':
				if brack > 0 {
					brack--
				}
			case '(':
				paren++
			case ')':
				if paren > 0 {
					paren--
				}
			case ',':
				if brace == 0 && brack == 0 && paren == 0 {
					goto haveElement
				}
			}
			i++
		}
	haveElement:
		valStr := strings.TrimSpace(raw[valStart:i])
		// Skip empty values but allow parsing to continue
		if valStr != "" {
			if v, ok := parseWSLValue(valStr); ok {
				arr = append(arr, v)
			}
			// Note: If parsing fails, we skip the element rather than fail the entire array
			// This matches the behavior of parseWSLObject
		}
		// skip trailing spaces and optional comma
		for i < n && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\t' || raw[i] == ',') {
			i++
		}
	}
	return arr, true
}
