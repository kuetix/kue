package transitions

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type templatesTransitions struct {
	workflow.BaseServiceTransition
	modulesPath   string
	workflowsPath string
	version       string
	buildTime     string
	fs            map[string]*flag.FlagSet
	commands      map[string]interface{}
}

func NewTemplatesTransition() interfaces.ServiceTransitions {
	return &templatesTransitions{}
}

//goland:noinspection GoUnusedParameter
func (t *templatesTransitions) TemplatesDownloadCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(t.GetSession(), config, flags)
		return
	}

	templateURL := options["template-url"].(string)
	templatePath := options["template-path"].(string)
	templateGit := options["template-git"].(string)
	templateVersion := options["template-version"].(string)

	if err := shared.InitializeTemplateManager(templateURL, templatePath, templateGit, templateVersion); err != nil {
		r.Error = fmt.Errorf("failed to download templates: %w", err)
		return
	}

	templatesDir := shared.TemplateManagerInstance.GetTemplatesDir()
	r.Success = true
	r.Response = fmt.Sprintf("Templates downloaded successfully to: %s\n", templatesDir)
	return
}

//goland:noinspection GoUnusedParameter
func (t *templatesTransitions) TemplatesUpdateCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(t.GetSession(), config, flags)
		return
	}

	templateURL := options["template-url"].(string)
	templatePath := options["template-path"].(string)
	templateGit := options["template-git"].(string)
	templateVersion := options["template-version"].(string)

	tm := shared.NewTemplateManagerFromFlags(templateURL, templatePath, templateGit, templateVersion)
	tm.SetForceRefresh(true)

	if err := tm.LoadTemplates(); err != nil {
		r.Error = fmt.Errorf("failed to update templates: %w", err)
		return
	}

	shared.TemplateManagerInstance = tm
	r.Success = true
	r.Response = fmt.Sprintf("Templates updated successfully to: %s\n", tm.GetTemplatesDir())
	return
}

//goland:noinspection GoUnusedParameter
func (t *templatesTransitions) TemplatesClearCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(t.GetSession(), config, flags)
		return
	}

	templateURL := options["template-url"].(string)
	templatePath := options["template-path"].(string)
	templateGit := options["template-git"].(string)
	templateVersion := options["template-version"].(string)

	tm := shared.NewTemplateManagerFromFlags(templateURL, templatePath, templateGit, templateVersion)
	cacheDir := tm.GetCacheDir()

	if err := os.RemoveAll(cacheDir); err != nil {
		r.Error = fmt.Errorf("failed to clear template cache at %s: %w", cacheDir, err)
		return
	}

	r.Success = true
	r.Response = fmt.Sprintf("Template cache cleared: %s\n", cacheDir)
	return
}

//goland:noinspection GoUnusedParameter
func (t *templatesTransitions) TemplatesStatusCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(t.GetSession(), config, flags)
		return
	}

	templateURL := options["template-url"].(string)
	templatePath := options["template-path"].(string)
	templateGit := options["template-git"].(string)
	templateVersion := options["template-version"].(string)

	tm := shared.NewTemplateManagerFromFlags(templateURL, templatePath, templateGit, templateVersion)
	cacheDir := tm.GetCacheDir()

	var sb strings.Builder
	sb.WriteString("Template Cache Status\n")
	sb.WriteString("=====================\n")
	sb.WriteString(fmt.Sprintf("Cache directory: %s\n", cacheDir))

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		sb.WriteString("Status: No cached templates found\n")
		r.Success = true
		r.Response = sb.String()
		return
	}

	// Check web cache
	webCacheDir := filepath.Join(cacheDir, "web")
	if info, err := os.Stat(webCacheDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(webCacheDir)
		if err == nil && len(entries) > 0 {
			sb.WriteString("\nWeb templates:\n")
			for _, entry := range entries {
				if entry.IsDir() {
					printCacheVersion(&sb, filepath.Join(webCacheDir, entry.Name()), "web", entry.Name())
				}
			}
		}
	}

	// Check git cache
	gitCacheDir := filepath.Join(cacheDir, "git")
	if info, err := os.Stat(gitCacheDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(gitCacheDir)
		if err == nil && len(entries) > 0 {
			sb.WriteString("\nGit templates:\n")
			for _, entry := range entries {
				if entry.IsDir() {
					printCacheVersion(&sb, filepath.Join(gitCacheDir, entry.Name()), "git", entry.Name())
				}
			}
		}
	}

	r.Success = true
	r.Response = sb.String()
	return
}

func printCacheVersion(sb *strings.Builder, path, sourceType, version string) {
	info, err := os.Stat(path)
	if err != nil {
		sb.WriteString(fmt.Sprintf("  - %s (%s): error reading cache\n", version, sourceType))
		return
	}
	sb.WriteString(fmt.Sprintf("  - %s (%s): cached, last modified %s\n", version, sourceType, info.ModTime().Format("2006-01-02 15:04:05")))
}
