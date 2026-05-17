package shared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ApplicationMetadata struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	Engine      string    `json:"engine"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WorkflowMetadata struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type FeatureMetadata struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SolutionMetadata struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PackageInfo struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Engine      string   `json:"engine"`
	Publisher   string   `json:"publisher"`
	Keywords    []string `json:"keywords"`
}

func CreateApplicationMetadata(projectPath, name, appType string) error {
	metadata := ApplicationMetadata{
		Name:        name,
		Type:        appType,
		Description: fmt.Sprintf("Kuetix Engine application: %s", name),
		Version:     "0.1.0",
		Engine:      KuetixEngineVersion,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal application metadata: %w", err)
	}
	metadataPath := filepath.Join(projectPath, "application.json")
	return os.WriteFile(metadataPath, data, 0644)
}

func CreateWorkflowMetadata(outputDir, subDir, name string) error {
	metadata := WorkflowMetadata{
		Name:        name,
		Type:        "workflow",
		Description: fmt.Sprintf("Workflow: %s", name),
		Version:     "0.1.0",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal workflow metadata: %w", err)
	}
	metadataPath := filepath.Join(outputDir, "workflows", subDir, name+".json")
	return os.WriteFile(metadataPath, data, 0644)
}

func CreateFeatureMetadata(outputDir, name string) error {
	metadata := FeatureMetadata{
		Name:        name,
		Type:        "feature",
		Description: fmt.Sprintf("Feature: %s", name),
		Version:     "0.1.0",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal feature metadata: %w", err)
	}
	metadataPath := filepath.Join(outputDir, "workflows/features", name+".json")
	return os.WriteFile(metadataPath, data, 0644)
}

func CreateSolutionMetadata(outputDir, name string) error {
	metadata := SolutionMetadata{
		Name:        name,
		Type:        "solution",
		Description: fmt.Sprintf("Solution: %s", name),
		Version:     "0.1.0",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal solution metadata: %w", err)
	}
	metadataPath := filepath.Join(outputDir, "workflows/solutions", name+".json")
	return os.WriteFile(metadataPath, data, 0644)
}

func ReadPackageInfo(pkgDir string) (PackageInfo, error) {
	var info PackageInfo
	data, err := os.ReadFile(filepath.Join(pkgDir, "kuetix.json"))
	if err != nil {
		return info, err
	}
	return info, json.Unmarshal(data, &info)
}

func ResolvePackageJSONPath(pathArg string) (string, error) {
	target := strings.TrimSpace(pathArg)
	if target == "" {
		target = "."
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("failed to access path '%s': %w", target, err)
	}
	if info.IsDir() {
		target = filepath.Join(target, "kuetix.json")
	}
	if filepath.Base(target) != "kuetix.json" {
		return "", fmt.Errorf("path '%s' must be a kuetix.json file or directory containing kuetix.json", pathArg)
	}
	if _, err := os.Stat(target); err != nil {
		return "", fmt.Errorf("kuetix.json not found at '%s': %w", target, err)
	}
	return target, nil
}
