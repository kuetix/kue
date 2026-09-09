package shared

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"github.com/kuetix/kue"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	DefaultTemplateURL  = "https://templates.kuetix.com/latest/"
	TemplateURLPattern  = "https://templates.kuetix.com/%s/"
	TemplateCacheDir    = "templates"
	KuetixEngineVersion = "v1.0.0"
	MinGoVersion        = "1.26.1"
)

// TemplateManagerInstance is the global template manager instance.
var TemplateManagerInstance *TemplateManager

// TemplateSource defines where templates are loaded from.
type TemplateSource struct {
	Type    string
	Path    string
	Version string
}

// TemplateManager handles template loading from various sources.
type TemplateManager struct {
	source       TemplateSource
	cacheDir     string
	templatesDir string
	forceRefresh bool
}

// NewTemplateManager creates a new template manager.
func NewTemplateManager(source TemplateSource) *TemplateManager {
	cacheDir := filepath.Join(kue.HomeDir, kue.CacheDir, TemplateCacheDir)
	return &TemplateManager{
		source:   source,
		cacheDir: cacheDir,
	}
}

func (tm *TemplateManager) SetForceRefresh(force bool) {
	tm.forceRefresh = force
}

func (tm *TemplateManager) LoadTemplates() error {
	switch tm.source.Type {
	case "web":
		return tm.loadFromWeb()
	case "local":
		return tm.loadFromLocal()
	case "git":
		return tm.loadFromGit()
	default:
		return fmt.Errorf("unknown template source type: %s", tm.source.Type)
	}
}

func (tm *TemplateManager) loadFromWeb() error {
	url := tm.source.Path
	if url == "" {
		if tm.source.Version != "" && tm.source.Version != "latest" {
			url = fmt.Sprintf(TemplateURLPattern, tm.source.Version)
		} else {
			url = DefaultTemplateURL
		}
	}
	version := tm.source.Version
	if version == "" {
		version = "latest"
	}
	versionCacheDir := filepath.Join(tm.cacheDir, "web", version)
	if _, err := os.Stat(versionCacheDir); err == nil {
		if !tm.forceRefresh {
			tm.templatesDir = versionCacheDir
			return nil
		}
		if err := os.RemoveAll(versionCacheDir); err != nil {
			return fmt.Errorf("failed to clear cached templates: %w", err)
		}
	}
	fmt.Printf("Downloading templates from %s...\n", url)
	if err := os.MkdirAll(versionCacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Try tar.gz first
	archiveURL := url
	if !strings.HasSuffix(archiveURL, "/") {
		archiveURL += "/"
	}
	archiveURL += "templates.tar.gz"

	resp, err := http.Get(archiveURL)
	if err == nil && resp.StatusCode == 200 {
		defer func() { _ = resp.Body.Close() }()
		tempFile, err := os.CreateTemp("", "templates-*.tar.gz")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tempPath := tempFile.Name()
		defer func() { _ = os.Remove(tempPath) }()
		if _, err = io.Copy(tempFile, resp.Body); err != nil {
			_ = tempFile.Close()
			return fmt.Errorf("failed to download templates: %w", err)
		}
		_ = tempFile.Close()
		if err := ExtractTarGz(tempPath, versionCacheDir); err != nil {
			return fmt.Errorf("failed to extract templates: %w", err)
		}
		tm.templatesDir = versionCacheDir
		fmt.Println("Templates downloaded and cached successfully")
		return nil
	}
	if resp != nil {
		_ = resp.Body.Close()
	}

	// Try zip
	archiveURL = url
	if !strings.HasSuffix(archiveURL, "/") {
		archiveURL += "/"
	}
	archiveURL += "templates.zip"

	resp, err = http.Get(archiveURL)
	if err == nil && resp.StatusCode == 200 {
		defer func() { _ = resp.Body.Close() }()
		tempFile, err := os.CreateTemp("", "templates-*.zip")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tempPath := tempFile.Name()
		defer func() { _ = os.Remove(tempPath) }()
		if _, err = io.Copy(tempFile, resp.Body); err != nil {
			_ = tempFile.Close()
			return fmt.Errorf("failed to download templates: %w", err)
		}
		_ = tempFile.Close()
		if err := ExtractZip(tempPath, versionCacheDir); err != nil {
			return fmt.Errorf("failed to extract templates: %w", err)
		}
		tm.templatesDir = versionCacheDir
		fmt.Println("Templates downloaded and cached successfully")
		return nil
	}
	if resp != nil {
		_ = resp.Body.Close()
	}

	return fmt.Errorf("failed to download templates from %s (tried .tar.gz and .zip)", url)
}

func (tm *TemplateManager) loadFromLocal() error {
	localPath := tm.source.Path
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("template directory not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("template path is not a directory: %s", localPath)
	}
	tm.templatesDir = localPath
	fmt.Printf("Using local templates from: %s\n", localPath)
	return nil
}

func (tm *TemplateManager) loadFromGit() error {
	gitURL := tm.source.Path
	sanitized := strings.ReplaceAll(gitURL, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	gitCacheDir := filepath.Join(tm.cacheDir, "git", sanitized)
	if _, err := os.Stat(filepath.Join(gitCacheDir, ".git")); err == nil {
		if tm.forceRefresh {
			fmt.Printf("Force refreshing templates from git repository: %s...\n", gitURL)
			if err := os.RemoveAll(gitCacheDir); err != nil {
				return fmt.Errorf("failed to clear cached git templates: %w", err)
			}
		} else {
			fmt.Printf("Updating templates from git repository...\n")
			cmd := exec.Command("git", "-C", gitCacheDir, "pull")
			if err := cmd.Run(); err != nil {
				fmt.Printf("Warning: Failed to update git repository, using cached version\n")
			}
			tm.templatesDir = gitCacheDir
			return nil
		}
	}
	fmt.Printf("Cloning templates from git repository: %s...\n", gitURL)
	if err := os.MkdirAll(filepath.Dir(gitCacheDir), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	cmd := exec.Command("git", "clone", gitURL, gitCacheDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone git repository: %w", err)
	}
	tm.templatesDir = gitCacheDir
	fmt.Println("Templates cloned successfully")
	return nil
}

func (tm *TemplateManager) GetTemplatePath(relativePath string) string {
	return filepath.Join(tm.templatesDir, relativePath)
}

func (tm *TemplateManager) ReadTemplate(relativePath string) ([]byte, error) {
	fullPath := tm.GetTemplatePath(relativePath)
	return os.ReadFile(fullPath)
}

func (tm *TemplateManager) GetCacheDir() string {
	return tm.cacheDir
}

func (tm *TemplateManager) GetTemplatesDir() string {
	return tm.templatesDir
}

func (tm *TemplateManager) ClearCache() error {
	return os.RemoveAll(tm.cacheDir)
}

// TemplateData holds data for template rendering.
type TemplateData struct {
	ProjectName         string
	AppType             string
	KuetixEngineVersion string
	MinGoVersion        string
	ModuleName          string
	ModuleNamePascal    string
	WorkflowName        string
	WorkflowNamePascal  string
	FeatureName         string
	FeatureNamePascal   string
	SolutionName        string
	SolutionNamePascal  string
	TransitionName      string
	MethodDescription   string
}

func ToCamelCase(s string) string {
	if s == "" {
		return ""
	}
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(words) == 0 {
		return ""
	}
	result := strings.ToLower(words[0])
	for i := 1; i < len(words); i++ {
		result += cases.Title(language.English).String(strings.ToLower(words[i]))
	}
	return result
}

func ToPascalCase(s string) string {
	if s == "" {
		return ""
	}
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(words) == 0 {
		return ""
	}
	var result string
	for _, word := range words {
		result += cases.Title(language.English).String(strings.ToLower(word))
	}
	return result
}

func ToSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(words) == 0 {
		return ""
	}
	var result string
	for i, word := range words {
		if i > 0 {
			result += "_"
		}
		result += strings.ToLower(word)
	}
	return result
}

func ToMethodName(s string) string {
	if s == "" {
		return ""
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			return ToPascalCase(s)
		}
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func RenderTemplate(templatePath string, data TemplateData) (string, error) {
	if TemplateManagerInstance == nil {
		return "", fmt.Errorf("template manager not initialized (run `kue templates download`)")
	}
	tmplContent, err := TemplateManagerInstance.ReadTemplate(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templatePath, err)
	}
	return buf.String(), nil
}

func CheckPathExistsAndConfirm(path string, force bool) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		if !force {
			fmt.Printf("Warning: '%s' already exists.\n", path)
			fmt.Print("Do you want to overwrite it? This may replace existing files. (yes/no): ")
			var response string
			_, _ = fmt.Scanln(&response)
			if response != "yes" && response != "y" && response != "Y" {
				return false, fmt.Errorf("operation cancelled by user")
			}
		}
		return true, nil
	} else if os.IsNotExist(err) {
		return true, nil
	} else {
		return false, fmt.Errorf("failed to check path: %w", err)
	}
}

func WriteTemplateToFile(templatePath, outputPath string, data TemplateData, perm os.FileMode) error {
	content, err := RenderTemplate(templatePath, data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", outputPath, err)
	}
	return os.WriteFile(outputPath, []byte(content), perm)
}

// InstallTemplateTree copies a whole template directory into projectPath:
// files ending in .tmpl are rendered with data (suffix stripped), all other
// files are copied verbatim. Used by full-app skeletons (templates/apps/cli, …).
func InstallTemplateTree(templateRelDir, projectPath string, data TemplateData) error {
	root := TemplateManagerInstance.GetTemplatePath(templateRelDir)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("template directory not found: %s (run 'kue templates update'?)", templateRelDir)
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Base(path) == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(rel, ".tmpl") {
			outPath := filepath.Join(projectPath, strings.TrimSuffix(rel, ".tmpl"))
			return WriteTemplateToFile(filepath.Join(templateRelDir, rel), outPath, data, 0644)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		outPath := filepath.Join(projectPath, rel)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(outPath, content, info.Mode().Perm())
	})
}

func ExtractTarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				_ = outFile.Close()
				return err
			}
			_ = outFile.Close()
		}
	}
	return nil
}

func ExtractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		path := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(path, f.Mode()); err != nil {
				_ = rc.Close()
				return err
			}
			_ = rc.Close()
			continue
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			_ = rc.Close()
			return err
		}
		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		_ = outFile.Close()
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// InitializeTemplateManager sets up the template manager based on source config.
func InitializeTemplateManager(templateURL, templatePath, templateGit, templateVersion string) error {
	var source TemplateSource
	if templatePath != "" {
		source = TemplateSource{Type: "local", Path: templatePath}
	} else if templateGit != "" {
		source = TemplateSource{Type: "git", Path: templateGit}
	} else {
		source = TemplateSource{Type: "web", Path: templateURL, Version: templateVersion}
		if templateVersion == "" {
			source.Version = "latest"
		}
	}
	TemplateManagerInstance = NewTemplateManager(source)
	return TemplateManagerInstance.LoadTemplates()
}

// NewTemplateManagerFromFlags builds a TemplateManager without loading templates.
func NewTemplateManagerFromFlags(templateURL, templatePath, templateGit, templateVersion string) *TemplateManager {
	var source TemplateSource
	if templatePath != "" {
		source = TemplateSource{Type: "local", Path: templatePath}
	} else if templateGit != "" {
		source = TemplateSource{Type: "git", Path: templateGit}
	} else {
		version := templateVersion
		if version == "" {
			version = "latest"
		}
		source = TemplateSource{Type: "web", Path: templateURL, Version: version}
	}
	return NewTemplateManager(source)
}
