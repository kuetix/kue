package caches

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	h "github.com/kuetix/engine/engine/helpers"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

func GenerateMetaCache(env *domain.Environment) {
	modulesPath, err := h.ModulesPath(env.Config.Application.ModulesPath)
	if err != nil {
		logger.Errorf("Failed to locate modules folder: %s: %s", err, env.Config.Application.ModulesPath)
		return
	}

	goModule, err := helpers.GetModuleFromGoMod()
	if err != nil {
		logger.Errorf("Failed to get module name from go.mod: %s", err)
		return
	}

	rootPath, err := helpers.GetRootPathGoMod()
	if err != nil {
		logger.Errorf("Failed to get module name from go.mod: %s", err)
		return
	}

	err = filepath.WalkDir(modulesPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			logger.Errorf("Error reading: %s", err)
			return err
		}

		if !d.IsDir() {
			return nil
		}

		// Check if a "transitions" subfolder exists here
		transitionsPath := filepath.Join(path, "transitions")
		if stat, err := os.Stat(transitionsPath); err == nil && stat.IsDir() {
			logger.Debugf("Walking in: %s", transitionsPath)

			return filepath.Walk(transitionsPath, func(subPath string, subInfo os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !subInfo.IsDir() && strings.HasSuffix(subPath, ".go") {
					logger.Debugf("Parsing file: %s", subPath)
					ParseGoFile(subPath, rootPath, modulesPath)
				}
				return nil
			})
		}
		return nil
	})

	if err != nil {
		logger.Errorf("Walk error: %s", err)
		return
	}

	// Generate the meta.go file
	if err = GenerateMetaFiles(goModule, modulesPath, "/"); err != nil {
		logger.Errorf("Failed to write meta.go: %s", err)
	}
}
