package helpers

import (
	"os"
	"path/filepath"
	"strings"
)

func ModulesPath(modulesPath string) (string, error) {
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()

	if modulesPath == "" {
		modulesPath = "./modules"
	}

	var osStat os.FileInfo
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	currentWd := ModulesImportPath(wd)
	parentRootWd := ModulesImportPath(filepath.Join(wd, ".."))
	var possiblePaths = []string{
		modulesPath,
		filepath.Join(currentWd, modulesPath),
		filepath.Join(parentRootWd, modulesPath),
		filepath.Join("../", modulesPath),
		filepath.Join("./core/", modulesPath),
		filepath.Join("./engine/", modulesPath),
		filepath.Join("./runtime/", modulesPath),
	}

	for _, path := range possiblePaths {
		if osStat, err = os.Stat(path); err == nil && osStat.IsDir() {
			modulesPath = path
			err = nil
			break
		} else {
			err = os.ErrNotExist
		}
	}

	if err != nil {
		return "", err
	}

	return modulesPath, err
}

// ModulesImportPath returns the path to the modules directory.
//
//goland:noinspection GoUnusedExportedFunction
func ModulesImportPath(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	relPath, err := os.Readlink(wd)
	if err != nil {
		relPath = wd
	}
	modulesImportPath := strings.Replace(path, relPath+"/", "", 1)

	return modulesImportPath
}

//goland:noinspection GoUnusedExportedFunction
func ReadTextFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return "err:" + err.Error()
	}
	return string(content)
}
