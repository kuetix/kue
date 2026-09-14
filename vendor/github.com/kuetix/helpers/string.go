package helpers

import (
	"fmt"
	"go/ast"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// FindStringIndex Function to find the index of a string in a slice of strings
// Returns -1 if the string is not found
//
//goland:noinspection GoUnusedExportedFunction
func FindStringIndex(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1 // Return -1 if the string is not found
}

type NaturalSort []string

func (ns NaturalSort) Len() int           { return len(ns) }
func (ns NaturalSort) Swap(i, j int)      { ns[i], ns[j] = ns[j], ns[i] }
func (ns NaturalSort) Less(i, j int) bool { return NaturalLess(ns[i], ns[j]) }

// NaturalLess compares two strings in natural order.
func NaturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ar, br := a[ai], b[bi]

		if unicode.IsDigit(rune(ar)) && unicode.IsDigit(rune(br)) {
			var an, bn int
			for an = ai; an < len(a) && unicode.IsDigit(rune(a[an])); an++ {
			}
			for bn = bi; bn < len(b) && unicode.IsDigit(rune(b[bn])); bn++ {
			}

			anum, _ := strconv.Atoi(a[ai:an])
			bnum, _ := strconv.Atoi(b[bi:bn])

			if anum != bnum {
				return anum < bnum
			}

			ai, bi = an, bn
		} else {
			if ar != br {
				return ar < br
			}
			ai++
			bi++
		}
	}
	return len(a) < len(b)
}

// EscapeRedisValue Function to escape a string for use in a Redis query
func EscapeRedisValue(query string) string {
	escaped := EscapeChars(query, "<>@_/\\+-.?()&^%$#@!=*", "\\")
	return escaped
}

// EscapeChars Function to escape a set of characters in a string
func EscapeChars(s string, charsToEscape string, escapeChar string) string {
	// Create a new builder for the result
	var escaped strings.Builder

	// Iterate through each character in the string
	for _, char := range s {
		// Check if the character is in the set of characters to escape
		if strings.ContainsRune(charsToEscape, char) {
			escaped.WriteString(escapeChar) // Add escape character
		}
		escaped.WriteRune(char) // Add the actual character
	}

	return escaped.String()
}

func IsString(i interface{}) bool {
	switch i.(type) {
	case string:
		return true
	}

	return false
}

func SplitString(item string, s string) (string, string) {
	split := strings.Split(item, s)
	if len(split) == 2 {
		return split[0], split[1]
	}
	return "", ""
}

func ToCamelCase(input string) string {
	words := strings.FieldsFunc(input, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, word := range words {
		words[i] = cases.Title(language.English).String(strings.ToLower(word))
	}
	return strings.Join(words, "")
}

func SanitizeServiceName(input string) string {
	input = strings.ReplaceAll(input, ".", "_")
	input = strings.ReplaceAll(input, "-", "_")
	input = strings.ReplaceAll(input, "/", "_")
	input = strings.ReplaceAll(input, "\\", "_")
	return input
}

// ToSnakeCase converts a string to snake_case
func ToSnakeCase(input string) string {
	var result strings.Builder
	for i, r := range input {
		if unicode.IsUpper(r) {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(r))
		} else if r == ' ' || r == '-' {
			result.WriteRune('_')
		} else {
			result.WriteRune(unicode.ToLower(r))
		}
	}
	return result.String()
}

func ExprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + ExprToString(e.X)
	case *ast.SelectorExpr:
		return ExprToString(e.X) + "." + e.Sel.Name
	case *ast.ArrayType:
		return "[]" + ExprToString(e.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.MapType:
		return "map[" + ExprToString(e.Key) + "]" + ExprToString(e.Value)
	default:
		return fmt.Sprintf("%T", expr) // fallback
	}
}
