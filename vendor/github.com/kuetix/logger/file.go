package logger

import (
	"path/filepath"
	"strings"
)

// RemoveCommonPrefix removes the common prefix from two paths.
// It returns the remaining paths.
//
//goland:noinspection GoUnusedExportedFunction
func RemoveCommonPrefix(a, b string) (string, string) {
	// Clean paths
	a = filepath.Clean(a)
	b = filepath.Clean(b)

	// Split into components
	pa := strings.Split(a, string(filepath.Separator))
	pb := strings.Split(b, string(filepath.Separator))

	// Find a common prefix index
	i := 0
	for i < len(pa) && i < len(pb) && pa[i] == pb[i] {
		i++
	}

	// Rejoin the remainders
	ra := strings.Join(pa[i:], "/")
	rb := strings.Join(pb[i:], "/")

	return ra, rb
}
