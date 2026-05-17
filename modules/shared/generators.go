package shared

import (
	"fmt"
	"path/filepath"
)

func GenerateCLIApp(projectPath, name string) error {
	data := TemplateData{
		ProjectName:         name,
		KuetixEngineVersion: KuetixEngineVersion,
		MinGoVersion:        MinGoVersion,
	}
	return WriteTemplateToFile(
		"templates/apps/cli/main.go.tmpl",
		filepath.Join(projectPath, "cmd/cli/main.go"),
		data, 0644,
	)
}

func GenerateAPIApp(projectPath, name string) error {
	data := TemplateData{
		ProjectName:         name,
		KuetixEngineVersion: KuetixEngineVersion,
		MinGoVersion:        MinGoVersion,
	}
	return WriteTemplateToFile(
		"templates/apps/api/main.go.tmpl",
		filepath.Join(projectPath, "cmd/api/main.go"),
		data, 0644,
	)
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
		"templates/apps/cli/main.go.tmpl",
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
	if err := WriteTemplateToFile(
		"templates/modules/modules.go.tmpl",
		filepath.Join(projectPath, "modules/modules.go"),
		data, 0644,
	); err != nil {
		return fmt.Errorf("failed to create modules/modules.go: %w", err)
	}
	return nil
}
