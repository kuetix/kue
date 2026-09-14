package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuetix/logger"
)

// TouchFile creates an empty file if it doesn't exist or updates the modification time if it does.
// It returns an error if the operation fails.
//
//goland:noinspection GoUnusedExportedFunction
func TouchFile(name string) error {
	file, err := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	return file.Close()
}

// Fprintf writes to the given writer using the format specifier format,
// substituting any instances of %s or %v according to the format specifiers,
// writing to os.Stdout if no writer is provided.
//
//goland:noinspection GoUnusedExportedFunction
func Fprintf(w *os.File, format string, args ...interface{}) (n int) {
	n, err := fmt.Fprintf(w, format, args...)
	if err != nil {
		logger.Errorf(fmt.Sprintf("Failed to write to file: %s", err))
		return -1
	}
	return n
}

// Fprintln writes the text to the given writer, followed by a newline character,
// writing to os.Stdout if no writer is provided.
//
//goland:noinspection GoUnusedExportedFunction
func Fprintln(w *os.File, format string) (n int) {
	n, err := fmt.Fprintln(w, format)
	if err != nil {
		logger.Errorf(fmt.Sprintf("Failed to write to file: %s", err))
		return -1
	}
	return n
}

// GetModuleFromGoMod returns the module name from the go.mod file.
//
//goland:noinspection GoUnusedExportedFunction
func GetModuleFromGoMod() (string, error) {
	goModFile, err := FindFileUp("go.mod")
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(goModFile)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") && !strings.HasPrefix(line, "//") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", os.ErrNotExist
}

// GetRootPathGoMod returns the root path of the Go module.
//
//goland:noinspection GoUnusedExportedFunction
func GetRootPathGoMod() (string, error) {
	goModFile, err := FindFileUp("go.mod")
	if err != nil {
		return "", err
	}

	moduleDir := filepath.Dir(goModFile)

	moduleRoot := moduleDir
	for {
		if _, err := os.Stat(filepath.Join(moduleRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(moduleRoot)
		if parent == moduleRoot {
			logger.Errorf(fmt.Sprintf("Cannot find module root for %s", moduleDir))
			return "", os.ErrNotExist
		}
		moduleRoot = parent
	}

	return moduleRoot, nil
}

// FindFileUp searches for a file in the current directory and its parent directories.
// It returns the path to the file if found, or an error if the file is not found.
//
//goland:noinspection GoUnusedExportedFunction
func FindFileUp(filename string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", os.ErrNotExist
}

// FindFileDown searches for a file in a directory and its subdirectories.
// It returns the path to the file if found, or an error if the file is not found.
//
//goland:noinspection GoUnusedExportedFunction
func FindFileDown(dir, filename string) (string, error) {
	var result string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == filename {
			result = path
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if result == "" {
		return "", os.ErrNotExist
	}
	return result, nil
}

// CleanSlashes removes leading and trailing slashes from a string.
//
//goland:noinspection GoUnusedExportedFunction
func CleanSlashes(paths ...string) []string {
	for _, part := range paths {
		part = strings.Trim(part, "/")
	}
	return paths
}
