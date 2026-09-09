package transitions

import (
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

// SearchWorkflowCommand calls the registry's public search endpoint
// (GET /workflows/search) and renders up to --limit results. No login is
// required: an anonymous caller sees public workflows, a logged-in one
// additionally sees their own private ones — the visibility rule is
// enforced server-side (workflowVisibleTo).
//
//goland:noinspection GoUnusedParameter
func (s *searchTransitions) SearchWorkflowCommand(command, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if options["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(s.GetSession(), config, flags)
		return
	}

	query := queryPositional(command, options)
	if query == "" {
		r.Success = true
		r.Response = RenderHelp(s.GetSession(), config, flags)
		return
	}
	limit := intOpt(options, "limit", 10)
	if limit <= 0 {
		limit = 10
	}

	hits, total, statusCode, err := searchWorkflows(kueConfig, query, limit)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("workflow search failed: %w", err)
		return
	}

	r.Success = true
	r.Response = renderWorkflowHits(query, hits, total, limit)
	return
}

// ---------------------------------------------------------------------------
// Workflow search
// ---------------------------------------------------------------------------

// workflowHit is one result from the registry's public search endpoint.
type workflowHit struct {
	Name              string
	Owner             string
	Project           string
	Version           int
	Public            bool
	ActionsCount      int
	DependenciesCount int
	MatchedActions    []string
}

// searchWorkflows calls GET /workflows/search?q=<query>&limit=<limit> — the
// same registry-wide, optional-auth endpoint the web UI's home search and
// pkg.kuetix.com use. PerformOptionalAuthRequest sends the stored token when
// one is present (so a logged-in caller's own private workflows are included
// alongside public ones) and goes out anonymously otherwise; either way the
// server enforces visibility, never this client.
func searchWorkflows(kueConfig shared.KueConfig, query string, limit int) ([]workflowHit, int, int, error) {
	urlPath := fmt.Sprintf("/workflows/search?q=%s&limit=%d", url.QueryEscape(query), limit)
	body, statusCode, err := shared.PerformOptionalAuthRequest(kueConfig, http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, 0, statusCode, err
	}
	hits, total, err := parseWorkflowSearchResponse(body)
	return hits, total, statusCode, err
}

// parseWorkflowSearchResponse decodes a /workflows/search response body into
// its hits and the server-reported total match count (which may exceed
// len(hits) once the requested --limit truncates the page).
func parseWorkflowSearchResponse(body string) ([]workflowHit, int, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, 0, nil
	}
	var obj struct {
		Data struct {
			Workflows []struct {
				Name              string   `json:"name"`
				Owner             string   `json:"owner"`
				Project           string   `json:"project"`
				Version           int      `json:"version"`
				Public            bool     `json:"public"`
				ActionsCount      int      `json:"actions_count"`
				DependenciesCount int      `json:"dependencies_count"`
				MatchedActions    []string `json:"matched_actions"`
			} `json:"workflows"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return nil, 0, fmt.Errorf("invalid JSON: %w", err)
	}
	hits := make([]workflowHit, 0, len(obj.Data.Workflows))
	for _, w := range obj.Data.Workflows {
		if strings.TrimSpace(w.Name) == "" {
			continue
		}
		hits = append(hits, workflowHit{
			Name:              w.Name,
			Owner:             w.Owner,
			Project:           w.Project,
			Version:           w.Version,
			Public:            w.Public,
			ActionsCount:      w.ActionsCount,
			DependenciesCount: w.DependenciesCount,
			MatchedActions:    w.MatchedActions,
		})
	}
	return hits, obj.Data.Total, nil
}

func renderWorkflowHits(query string, hits []workflowHit, total, limit int) string {
	var sb strings.Builder
	if len(hits) == 0 {
		sb.WriteString(fmt.Sprintf("No workflows match %q\n", query))
		return sb.String()
	}
	noun := "matches"
	if len(hits) == 1 {
		noun = "match"
	}
	sb.WriteString(fmt.Sprintf("Found %d %s for %q (showing up to %d of %d total):\n", len(hits), noun, query, limit, total))
	for _, h := range hits {
		visibility := "private"
		if h.Public {
			visibility = "public"
		}
		line := fmt.Sprintf("  - %s  v%d  (%s", h.Name, h.Version, visibility)
		if h.Owner != "" {
			line += ", owner=" + h.Owner
		}
		line += ")"
		sb.WriteString(line + "\n")
		if h.Project != "" {
			sb.WriteString("      project: " + h.Project + "\n")
		}
		if len(h.MatchedActions) > 0 {
			sb.WriteString("      matched actions: " + strings.Join(h.MatchedActions, ", ") + "\n")
		}
		sb.WriteString("      page:    " + workflowWebURL(h.Name, h.Owner) + "\n")
	}
	return sb.String()
}

// workflowWebURL is the shareable pkg.kuetix.com page for a registry workflow.
func workflowWebURL(name, owner string) string {
	u := "https://pkg.kuetix.com/workflows/" + name
	if owner != "" {
		u += "?owner=" + owner
	}
	return u
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func firstPositional(config map[string]interface{}, options map[string]interface{}) string {
	if args, ok := config["args"].([]string); ok && len(args) > 0 {
		return strings.TrimSpace(args[0])
	}
	return ""
}

// queryPositional returns the search query for `kue wsl <query>`.
//
//   - `--search`/`-s` is explicit and wins: it disambiguates a query that
//     collides with a subcommand name (`kue wsl --search get`) and makes the
//     intent obvious in scripts/CI.
//   - Otherwise a single bare token is parsed by the CLI as the command
//     "wsl.<query>" with an empty args list (GetArgs always treats the second
//     token as a subcommand), so recover it from the raw requested-command
//     string.
//   - The `kue search package <q>` form still lands normally in config["args"].
func queryPositional(requested map[string]interface{}, options map[string]interface{}) string {
	if s, _ := options["search"].(string); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if q := firstPositional(requested, options); q != "" {
		return q
	}
	full, _ := requested["command"].(string)
	main, _ := requested["main_command"].(string)
	if main != "" && strings.HasPrefix(full, main+".") {
		if t := strings.TrimSpace(strings.TrimPrefix(full, main+".")); t != "" && t != "*" {
			return t
		}
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
