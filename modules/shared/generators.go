package shared

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateCLIApp installs the full CLI skeleton (cmd/, embed.go, modules/,
// workflows/) mirrored from the kue source into projectPath.
func GenerateCLIApp(projectPath, name string) error {
	data := TemplateData{
		ProjectName:         name,
		KuetixEngineVersion: KuetixEngineVersion,
		MinGoVersion:        MinGoVersion,
	}
	return InstallTemplateTree("templates/apps/cli", projectPath, data)
}

// GenerateAPIApp installs the full API-server skeleton (cmd/, embed.go,
// internal/, modules/, workflows/) mirrored from the kuetix/api source.
func GenerateAPIApp(projectPath, name string) error {
	data := TemplateData{
		ProjectName:         name,
		KuetixEngineVersion: KuetixEngineVersion,
		MinGoVersion:        MinGoVersion,
	}
	return InstallTemplateTree("templates/apps/api", projectPath, data)
}

func GenerateConsumerApp(projectPath, name string) error {
	data := TemplateData{
		ProjectName:         name,
		KuetixEngineVersion: KuetixEngineVersion,
		MinGoVersion:        MinGoVersion,
	}
	return WriteTemplateToFile(
		"templates/apps/consumer/main.go.tmpl",
		filepath.Join(projectPath, "cmd/consumer/main.go"),
		data, 0644,
	)
}

func GenerateServiceApp(projectPath, name string) error {
	data := TemplateData{
		ProjectName:         name,
		KuetixEngineVersion: KuetixEngineVersion,
		MinGoVersion:        MinGoVersion,
	}
	return WriteTemplateToFile(
		"templates/apps/service/main.go.tmpl",
		filepath.Join(projectPath, "cmd/service/main.go"),
		data, 0644,
	)
}

func GeneratePackageSkeleton(projectPath, name string) error {
	data := TemplateData{
		ProjectName:         name,
		KuetixEngineVersion: KuetixEngineVersion,
		MinGoVersion:        MinGoVersion,
		ModuleName:          "example",
		ModuleNamePascal:    "Example",
		WorkflowName:        "example_workflow",
		WorkflowNamePascal:  "Example",
		FeatureName:         "example_feature",
		FeatureNamePascal:   "ExampleFeature",
		SolutionName:        "example_solution",
		SolutionNamePascal:  "ExampleSolution",
	}
	if err := WriteTemplateToFile(
		"templates/config/kuetix.json.tmpl",
		filepath.Join(projectPath, "kuetix.json"),
		data, 0644,
	); err != nil {
		return fmt.Errorf("failed to create kuetix.json: %w", err)
	}
	if err := WriteTemplateToFile(
		"templates/apps/pkg/main.go.tmpl",
		filepath.Join(projectPath, "cmd/pkg/main.go"),
		data, 0644,
	); err != nil {
		return fmt.Errorf("failed to create cmd/pkg/main.go: %w", err)
	}
	return nil
}

func GenerateCommonFiles(projectPath, name, appType string) error {
	data := TemplateData{
		ProjectName:         name,
		AppType:             appType,
		KuetixEngineVersion: KuetixEngineVersion,
		MinGoVersion:        MinGoVersion,
	}
	if err := WriteTemplateToFile(
		"templates/config/go.mod.tmpl",
		filepath.Join(projectPath, "go.mod"),
		data, 0644,
	); err != nil {
		return fmt.Errorf("failed to create go.mod: %w", err)
	}
	if err := WriteTemplateToFile(
		"templates/config/gitignore.tmpl",
		filepath.Join(projectPath, ".gitignore"),
		data, 0644,
	); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}
	if err := WriteTemplateToFile(
		"templates/config/Makefile.tmpl",
		filepath.Join(projectPath, "Makefile"),
		data, 0644,
	); err != nil {
		return fmt.Errorf("failed to create Makefile: %w", err)
	}
	readmeTemplate := "templates/config/README-app.md.tmpl"
	if appType == "package" {
		readmeTemplate = "templates/config/README-package.md.tmpl"
	}
	if err := WriteTemplateToFile(
		readmeTemplate,
		filepath.Join(projectPath, "README.md"),
		data, 0644,
	); err != nil {
		return fmt.Errorf("failed to create README.md: %w", err)
	}
	if err := WriteTemplateToFile(
		"templates/config/Dockerfile.tmpl",
		filepath.Join(projectPath, "Dockerfile"),
		data, 0644,
	); err != nil {
		return fmt.Errorf("failed to create Dockerfile: %w", err)
	}
	if err := WriteTemplateToFile(
		"templates/config/docker-compose.yml.tmpl",
		filepath.Join(projectPath, "docker-compose.yml"),
		data, 0644,
	); err != nil {
		return fmt.Errorf("failed to create docker-compose.yml: %w", err)
	}
	// Full app skeletons (cli, api) ship their own modules/modules.go —
	// don't overwrite it with the generic one.
	modulesGoPath := filepath.Join(projectPath, "modules/modules.go")
	if _, err := os.Stat(modulesGoPath); os.IsNotExist(err) {
		if err := WriteTemplateToFile(
			"templates/modules/modules.go.tmpl",
			modulesGoPath,
			data, 0644,
		); err != nil {
			return fmt.Errorf("failed to create modules/modules.go: %w", err)
		}
	}
	return nil
}
