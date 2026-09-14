package transitions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/spf13/cast"
)

// dropWSLArrayTail counteracts an engine@v1.0.0 quirk: an array literal in a
// WSL `const {}` block (or passed as an action arg) arrives with its last
// element duplicated once. When the final two entries are deep-equal we drop
// the last one. Two genuinely identical trailing rules/thresholds are
// meaningless anyway, and validateRuleset still rejects real duplicate ids.
func dropWSLArrayTail[T any](items []T) []T {
	n := len(items)
	if n >= 2 && reflect.DeepEqual(items[n-1], items[n-2]) {
		return items[:n-1]
	}
	return items
}

// Conflict-resolution policies. The policy IS the conflict-resolution
// strategy - it is declared in the ruleset, never inferred.
const (
	policyFirstMatch = "first-match"
	policyPriority   = "priority"
	policyAllMatches = "all-matches"
	policyScore      = "score"
)

// InputSpec is one entry of a ruleset's optional `inputs` schema. It drives
// facts.Prepare (coercion / defaults / required checks) so every rule state-group
// works on clean, like-typed facts.
type InputSpec struct {
	Type     string      `json:"type"`
	Required bool        `json:"required"`
	Default  interface{} `json:"default"`
}

// RuleRef is a rule's metadata in the ruleset. The rule's condition logic
// is not here - it lives in the decision workflow as a group of states
// (decision/cond.* checks composed with WSL control flow) that ends on
// decision/rule.Fire(id: "<this id>") or decision/rule.Skip(id: "<this id>").
//
// Then / Because are what a bare Fire (one with no outcome/reason of its
// own) votes for and records; Weight is the score-policy contribution.
type RuleRef struct {
	ID       string  `json:"id"`
	Priority int     `json:"priority"`
	Then     string  `json:"then"`
	Because  string  `json:"because"`
	Weight   float64 `json:"weight"`
}

// Threshold maps an accumulated score to an outcome (score policy only).
// The highest Min that is <= the score wins.
type Threshold struct {
	Min     float64 `json:"min"`
	Outcome string  `json:"outcome"`
}

// Ruleset is the canonical decision definition - typically written as an
// inline WSL `const {}` block and passed as $constants. It names an ordered
// list of rules and the policy that resolves their verdicts into a
// single outcome.
type Ruleset struct {
	Name           string                            `json:"name"`
	Version        string                            `json:"version"`
	Policy         string                            `json:"policy"`
	DefaultOutcome string                            `json:"defaultOutcome"`
	Inputs         map[string]InputSpec              `json:"inputs"`
	Rules          []RuleRef                         `json:"rules"`
	Thresholds     []Threshold                       `json:"thresholds"`
	OnOutcome      map[string]map[string]interface{} `json:"onOutcome"`
}

type rulesetTransitions struct {
	workflow.BaseServiceTransition
}

// NewRulesetTransitions is the DI constructor for the `decision/ruleset` class.
func NewRulesetTransitions() interfaces.ServiceTransitions {
	return &rulesetTransitions{}
}

// resolveRulesetDir is the directory bare ruleset names resolve against.
// Override with DECISION_RULESET_DIR.
func resolveRulesetDir() string {
	if d := strings.TrimSpace(os.Getenv("DECISION_RULESET_DIR")); d != "" {
		return d
	}
	return "decisions"
}

// loadRuleset accepts a ruleset from wherever the caller has it: an inline
// object (WSL `const {}` / a variable), a JSON string, a name resolved to
// <DECISION_RULESET_DIR>/<name>.json, or an explicit path.
func loadRuleset(source interface{}) (Ruleset, error) {
	switch v := source.(type) {
	case nil:
		return Ruleset{}, fmt.Errorf("ruleset is required")
	case Ruleset:
		return normalizeRuleset(v), nil
	case map[string]interface{}:
		return rulesetFromMap(v)
	case string:
		return loadRulesetFromString(v)
	default:
		return Ruleset{}, fmt.Errorf("ruleset must be an object, a JSON string, a name, or a path (got %T)", source)
	}
}

func loadRulesetFromString(s string) (Ruleset, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return Ruleset{}, fmt.Errorf("ruleset is required")
	}
	if strings.HasPrefix(trimmed, "{") {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return Ruleset{}, fmt.Errorf("ruleset is not valid JSON: %w", err)
		}
		return rulesetFromMap(raw)
	}

	path := trimmed
	looksLikePath := strings.ContainsAny(trimmed, "/\\") || strings.HasSuffix(trimmed, ".json")
	if !looksLikePath {
		path = filepath.Join(resolveRulesetDir(), trimmed+".json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Ruleset{}, fmt.Errorf("could not read ruleset %q: %w", path, err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Ruleset{}, fmt.Errorf("ruleset file %q is not valid JSON: %w", path, err)
	}
	rs, err := rulesetFromMap(raw)
	if err != nil {
		return rs, err
	}
	if rs.Name == "" {
		rs.Name = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	return rs, nil
}

// rulesetFromMap round-trips an untyped map (a WSL object / parsed JSON)
// through the typed Ruleset via JSON so nested rules/inputs decode cleanly.
func rulesetFromMap(raw map[string]interface{}) (Ruleset, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return Ruleset{}, fmt.Errorf("ruleset is not serialisable: %w", err)
	}
	var rs Ruleset
	if err := json.Unmarshal(encoded, &rs); err != nil {
		return Ruleset{}, fmt.Errorf("ruleset shape is invalid: %w", err)
	}
	return normalizeRuleset(rs), nil
}

func normalizeRuleset(rs Ruleset) Ruleset {
	rs.Policy = strings.TrimSpace(strings.ToLower(rs.Policy))
	if rs.Policy == "" {
		rs.Policy = policyFirstMatch
	}
	rs.DefaultOutcome = strings.TrimSpace(rs.DefaultOutcome)

	rs.Rules = dropWSLArrayTail(rs.Rules)
	rs.Thresholds = dropWSLArrayTail(rs.Thresholds)

	for i := range rs.Rules {
		rs.Rules[i].ID = strings.TrimSpace(rs.Rules[i].ID)
		rs.Rules[i].Then = strings.TrimSpace(rs.Rules[i].Then)
		rs.Rules[i].Because = strings.TrimSpace(rs.Rules[i].Because)
		if rs.Rules[i].ID == "" {
			rs.Rules[i].ID = fmt.Sprintf("rule-%d", i+1)
		}
	}
	return rs
}

// validateRuleset is the shared static check behind ruleset.Validate.
func validateRuleset(rs Ruleset) (errs []string, warnings []string) {
	switch rs.Policy {
	case policyFirstMatch, policyPriority, policyAllMatches, policyScore:
	default:
		errs = append(errs, fmt.Sprintf("unknown policy %q (want first-match, priority, all-matches, or score)", rs.Policy))
	}
	if len(rs.Rules) == 0 {
		errs = append(errs, "ruleset has no rules")
	}
	if rs.Policy == policyScore {
		if len(rs.Thresholds) == 0 {
			errs = append(errs, "score policy requires at least one threshold")
		}
		if rs.DefaultOutcome == "" && !thresholdCoversZero(rs.Thresholds) {
			warnings = append(warnings, "score policy has no defaultOutcome and no threshold at min 0: an all-skip evaluation has no outcome")
		}
	} else if rs.DefaultOutcome == "" {
		warnings = append(warnings, "no defaultOutcome: an evaluation where every rule skips returns an empty outcome")
	}

	seenID := map[string]bool{}
	seenPriority := map[int]string{}
	for _, rule := range rs.Rules {
		if seenID[rule.ID] {
			errs = append(errs, fmt.Sprintf("duplicate rule id %q", rule.ID))
		}
		seenID[rule.ID] = true

		switch rs.Policy {
		case policyScore:
			if rule.Weight == 0 {
				warnings = append(warnings, fmt.Sprintf("rule %q has weight 0 under the score policy: it never affects the outcome", rule.ID))
			}
		default:
			if rule.Then == "" {
				warnings = append(warnings, fmt.Sprintf("rule %q has no `then`: it relies on its Fire state passing(outcome: ...)", rule.ID))
			}
		}

		if rs.Policy == policyPriority {
			if other, clash := seenPriority[rule.Priority]; clash {
				warnings = append(warnings, fmt.Sprintf("rules %q and %q share priority %d: ties break by rule order", other, rule.ID, rule.Priority))
			}
			seenPriority[rule.Priority] = rule.ID
		}
	}
	return errs, warnings
}

func thresholdCoversZero(ts []Threshold) bool {
	for _, t := range ts {
		if t.Min <= 0 {
			return true
		}
	}
	return false
}

func rulesetToMap(rs Ruleset) map[string]interface{} {
	encoded, _ := json.Marshal(rs)
	var out map[string]interface{}
	_ = json.Unmarshal(encoded, &out)
	return out
}

func sortedRuleIDs(rs Ruleset) []string {
	ids := make([]string, 0, len(rs.Rules))
	for _, rule := range rs.Rules {
		ids = append(ids, rule.ID)
	}
	sort.Strings(ids)
	return ids
}

// castValue coerces a raw fact to the type declared for it in `inputs`.
func castValue(raw interface{}, declared string) (interface{}, error) {
	switch strings.TrimSpace(strings.ToLower(declared)) {
	case "number", "int", "integer", "float":
		return cast.ToFloat64E(raw)
	case "bool", "boolean":
		return cast.ToBoolE(raw)
	case "string", "text":
		return cast.ToStringE(raw)
	default:
		return raw, nil
	}
}

// Load reads a ruleset from an inline object, a JSON string, a name, or a
// path, normalises it, and returns the canonical form. It does not run
// anything.
func (t *rulesetTransitions) Load(ruleset interface{}) (r domain.FlowStepResult) {
	rs, err := loadRuleset(ruleset)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"ruleset": rulesetToMap(rs),
		"name":    rs.Name,
		"policy":  rs.Policy,
		"rules":   len(rs.Rules),
	}
	return
}

// Validate statically checks a ruleset: the policy is known, every rule
// names a workflow, ids are unique, and it flags missing `then`, duplicate
// priorities, and score-policy gaps as warnings. Errors are reported via
// r.Error so a workflow's `on fail` branch fires. This is the check a
// publish pipeline runs.
func (t *rulesetTransitions) Validate(ruleset interface{}) (r domain.FlowStepResult) {
	rs, err := loadRuleset(ruleset)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}
	errs, warnings := validateRuleset(rs)
	if len(errs) > 0 {
		r.Error = fmt.Errorf("ruleset %q is invalid: %s", rs.Name, strings.Join(errs, "; "))
		r.StatusCode = http.StatusUnprocessableEntity
		r.Response = map[string]interface{}{"valid": false, "name": rs.Name, "errors": errs, "warnings": warnings}
		return
	}
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"valid":    true,
		"name":     rs.Name,
		"policy":   rs.Policy,
		"rules":    len(rs.Rules),
		"ruleIds":  sortedRuleIDs(rs),
		"warnings": warnings,
	}
	return
}
