package transitions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type searchTransitions struct {
	workflow.BaseServiceTransition
}

func NewSearchTransition() interfaces.ServiceTransitions { return &searchTransitions{} }

// SearchWorkflowCommand paginates /workflow on the registry and returns the
// names that match the query as a case-insensitive substring, capped at the
// --limit value (default 10).
//
//goland:noinspection GoUnusedParameter
func (s *searchTransitions) SearchWorkflowCommand(command, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if options["help"].(bool) {
		r.Success = true
		r.Response = renderHelp(config, flagSet)
		return
	}

	query := firstPositional(command, options)
	if query == "" {
		r.Error = fmt.Errorf("query is required (usage: kue search wsl <query>)")
		return
	}
	limit := intOpt(options, "limit", 10)
	if limit <= 0 {
		limit = 10
	}

	matches, scanned, statusCode, err := searchWorkflows(kueConfig, query, limit)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("workflow search failed: %w", err)
		return
	}

	r.Success = true
	r.Response = renderWorkflowMatches(query, matches, scanned, limit)
	return
}

// SearchPackageCommand calls /packages/search?q=<query> on the registry and
// returns up to --limit results.
//
//goland:noinspection GoUnusedParameter
func (s *searchTransitions) SearchPackageCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if options["help"].(bool) {
		r.Success = true
		r.Response = renderHelp(config, flagSet)
		return
	}

	query := firstPositional(config, options)
	if query == "" {
		r.Error = fmt.Errorf("query is required (usage: kue search package <query>)")
		return
	}
	limit := intOpt(options, "limit", 10)
	if limit <= 0 {
		limit = 10
	}

	body, statusCode, err := shared.PerformAuthenticatedRequest(kueConfig, http.MethodGet, "/packages/search?q="+url.QueryEscape(query), nil)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("package search failed: %w", err)
		return
	}

	results := parsePackageResults(body)
	r.Success = true
	r.Response = renderPackageMatches(query, results, limit, body)
	return
}

// ---------------------------------------------------------------------------
// Workflow search
// ---------------------------------------------------------------------------

const (
	workflowSearchPageSize = 100
	workflowSearchMaxPages = 50
)

func searchWorkflows(kueConfig shared.KueConfig, query string, limit int) ([]string, int, int, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	var matches []string
	seen := map[string]bool{}
	cursor := ""
	scanned := 0
	var lastStatus int

	for page := 0; page < workflowSearchMaxPages; page++ {
		urlPath := fmt.Sprintf("/workflow?limit=%d&cursor=%s", workflowSearchPageSize, url.QueryEscape(cursor))
		body, statusCode, err := shared.PerformAuthenticatedRequest(kueConfig, http.MethodGet, urlPath, nil)
		lastStatus = statusCode
		if err != nil {
			return matches, scanned, statusCode, err
		}

		names, nextCursor, err := parseWorkflowListPage(body)
		if err != nil {
			return matches, scanned, statusCode, err
		}
		for _, name := range names {
			scanned++
			if seen[name] {
				continue
			}
			if needle == "" || strings.Contains(strings.ToLower(name), needle) {
				seen[name] = true
				matches = append(matches, name)
				if len(matches) >= limit {
					return matches, scanned, statusCode, nil
				}
			}
		}
		if nextCursor == "" || nextCursor == cursor {
			break
		}
		cursor = nextCursor
	}
	return matches, scanned, lastStatus, nil
}

// parseWorkflowListPage extracts workflow names and the next cursor from one
// page of the /workflow list endpoint. Returns names, next cursor, error.
func parseWorkflowListPage(body string) ([]string, string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, "", nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return nil, "", fmt.Errorf("invalid JSON: %w", err)
	}

	var names []string
	for _, key := range []string{"workflows", "items", "data", "results"} {
		if v, ok := obj[key]; ok {
			if arr, ok := v.([]interface{}); ok {
				names = append(names, namesFromArray(arr)...)
			}
		}
	}

	nextCursor := ""
	if data, ok := obj["data"].(map[string]interface{}); ok {
		for _, key := range []string{"workflows", "items", "results"} {
			if v, ok := data[key]; ok {
				if arr, ok := v.([]interface{}); ok {
					names = append(names, namesFromArray(arr)...)
				}
			}
		}
		if c, ok := data["cursor"].(string); ok && strings.TrimSpace(c) != "" {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(c))
			if err == nil && string(decoded) != "" && string(decoded) != "|" {
				nextCursor = c
			}
		}
	}
	return names, nextCursor, nil
}

func namesFromArray(items []interface{}) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch v := it.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				out = append(out, s)
			}
		case map[string]interface{}:
			for _, key := range []string{"name", "workflow", "id"} {
				if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
					break
				}
			}
		}
	}
	return out
}

func renderWorkflowMatches(query string, matches []string, scanned, limit int) string {
	var sb strings.Builder
	if len(matches) == 0 {
		sb.WriteString(fmt.Sprintf("No workflows match %q (scanned %d names)\n", query, scanned))
		return sb.String()
	}
	noun := "matches"
	if len(matches) == 1 {
		noun = "match"
	}
	sb.WriteString(fmt.Sprintf("Found %d %s for %q (showing up to %d, scanned %d):\n", len(matches), noun, query, limit, scanned))
	for _, m := range matches {
		sb.WriteString("  - " + m + "\n")
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Package search
// ---------------------------------------------------------------------------

type packageHit struct {
	Name        string
	Version     string
	Description string
	Publisher   string
}

func parsePackageResults(body string) []packageHit {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	var raw interface{}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil
	}
	items := pluckPackageArray(raw)
	out := make([]packageHit, 0, len(items))
	for _, it := range items {
		entry, ok := it.(map[string]interface{})
		if !ok {
			if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, packageHit{Name: strings.TrimSpace(s)})
			}
			continue
		}
		hit := packageHit{
			Name:        strFromMap(entry, "name"),
			Version:     strFromMap(entry, "version"),
			Description: strFromMap(entry, "description"),
			Publisher:   strFromMap(entry, "publisher"),
		}
		if hit.Name == "" {
			continue
		}
		out = append(out, hit)
	}
	return out
}

func pluckPackageArray(raw interface{}) []interface{} {
	switch v := raw.(type) {
	case []interface{}:
		return v
	case map[string]interface{}:
		for _, key := range []string{"packages", "items", "results", "data"} {
			if arr, ok := v[key].([]interface{}); ok {
				return arr
			}
			if inner, ok := v[key].(map[string]interface{}); ok {
				for _, k2 := range []string{"packages", "items", "results"} {
					if arr, ok := inner[k2].([]interface{}); ok {
						return arr
					}
				}
			}
		}
	}
	return nil
}

func renderPackageMatches(query string, results []packageHit, limit int, rawBody string) string {
	if len(results) == 0 {
		body := strings.TrimSpace(rawBody)
		if body == "" {
			return fmt.Sprintf("No packages match %q\n", query)
		}
		return fmt.Sprintf("No packages match %q. Raw response:\n%s\n", query, body)
	}
	if len(results) > limit {
		results = results[:limit]
	}
	var sb strings.Builder
	noun := "matches"
	if len(results) == 1 {
		noun = "match"
	}
	sb.WriteString(fmt.Sprintf("Found %d %s for %q (showing up to %d):\n", len(results), noun, query, limit))
	for _, h := range results {
		line := "  - " + h.Name
		if h.Version != "" {
			line += "  " + h.Version
		}
		if h.Publisher != "" {
			line += "  (by " + h.Publisher + ")"
		}
		sb.WriteString(line + "\n")
		if h.Description != "" {
			sb.WriteString("      " + h.Description + "\n")
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func renderHelp(config map[string]interface{}, flagSet *flag.FlagSet) string {
	usage, _ := config["usage"].(string)
	var buf bytes.Buffer
	flagSet.SetOutput(&buf)
	flagSet.Usage()
	return usage + "\n" + buf.String()
}

func firstPositional(config map[string]interface{}, options map[string]interface{}) string {
	if args, ok := config["args"].([]string); ok && len(args) > 0 {
		return strings.TrimSpace(args[0])
	}
	return ""
}

func intOpt(options map[string]interface{}, key string, fallback int) int {
	switch v := options[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if v == "" {
			return fallback
		}
	}
	return fallback
}

func strFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
