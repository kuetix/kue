package transitions

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type engineTransitions struct {
	workflow.BaseServiceTransition
}

// NewEngineTransitions is the DI constructor for the `decision/engine` class.
func NewEngineTransitions() interfaces.ServiceTransitions {
	return &engineTransitions{}
}

// ruleEvaluation is one row of the audit trail.
type ruleEvaluation struct {
	ID        string  `json:"id"`
	Evaluated bool    `json:"evaluated"`
	Fired     bool    `json:"fired"`
	Outcome   string  `json:"outcome,omitempty"`
	Reason    string  `json:"reason,omitempty"`
	Weight    float64 `json:"weight,omitempty"`
}

type decisionResult struct {
	Outcome  string
	Outcomes []string
	Fired    []string
	Score    float64
	Policy   string
	Ruleset  string
	Trace    []ruleEvaluation
}

type verdict struct {
	fired   bool
	outcome string
	reason  string
}

// collectVerdicts reads the slice that each rule's decision/rule.Fire /
// decision/rule.Skip appended to the shared context and keys it by rule id.
func collectVerdicts(p *workflow.WorkerSessionContext) map[string]verdict {
	out := map[string]verdict{}
	if p == nil {
		return out
	}
	raw, ok := p.Value(verdictsKey).([]interface{})
	if !ok {
		return out
	}
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id == "" {
			continue
		}
		f, _ := m["fired"].(bool)
		o, _ := m["outcome"].(string)
		rs, _ := m["reason"].(string)
		out[id] = verdict{fired: f, outcome: o, reason: rs}
	}
	return out
}

// resolve is the branching core - the one place N rule verdicts are turned
// into a single outcome under a policy. Pure given the ruleset and the
// verdict set.
func resolve(rs Ruleset, verdicts map[string]verdict) (decisionResult, error) {
	trace := make([]ruleEvaluation, 0, len(rs.Rules))
	firedIDs := make([]string, 0, len(rs.Rules))
	firedOutcomes := make([]string, 0, len(rs.Rules))
	var score float64

	type firedRule struct {
		ref     RuleRef
		outcome string
	}
	fired := make([]firedRule, 0, len(rs.Rules))

	for _, rule := range rs.Rules {
		row := ruleEvaluation{ID: rule.ID, Weight: rule.Weight}
		v, seen := verdicts[rule.ID]
		row.Evaluated = seen
		if !seen || !v.fired {
			trace = append(trace, row)
			continue
		}

		outcome := v.outcome
		if outcome == "" {
			outcome = rule.Then
		}
		reason := v.reason
		if reason == "" {
			reason = rule.Because
		}
		row.Fired = true
		row.Outcome = outcome
		row.Reason = reason
		trace = append(trace, row)

		firedIDs = append(firedIDs, rule.ID)
		fired = append(fired, firedRule{ref: rule, outcome: outcome})
		if outcome != "" {
			firedOutcomes = append(firedOutcomes, outcome)
		}
		score += rule.Weight
	}

	result := decisionResult{
		Outcomes: dedupe(firedOutcomes),
		Fired:    firedIDs,
		Score:    score,
		Policy:   rs.Policy,
		Ruleset:  rs.Name,
		Trace:    trace,
	}

	switch rs.Policy {
	case policyFirstMatch:
		if len(fired) > 0 {
			result.Outcome = fired[0].outcome
		} else {
			result.Outcome = rs.DefaultOutcome
		}
	case policyPriority:
		if len(fired) > 0 {
			best := fired[0]
			for _, f := range fired[1:] {
				if f.ref.Priority > best.ref.Priority {
					best = f
				}
			}
			result.Outcome = best.outcome
		} else {
			result.Outcome = rs.DefaultOutcome
		}
	case policyAllMatches:
		if len(result.Outcomes) > 0 {
			result.Outcome = result.Outcomes[0]
		} else {
			result.Outcome = rs.DefaultOutcome
		}
	case policyScore:
		result.Outcome = outcomeForScore(rs, score)
	default:
		return result, fmt.Errorf("unknown policy %q", rs.Policy)
	}
	return result, nil
}

func outcomeForScore(rs Ruleset, score float64) string {
	sorted := append([]Threshold(nil), rs.Thresholds...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Min < sorted[j].Min })
	outcome := rs.DefaultOutcome
	for _, threshold := range sorted {
		if score >= threshold.Min {
			outcome = strings.TrimSpace(threshold.Outcome)
		}
	}
	return outcome
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func traceToInterfaces(trace []ruleEvaluation) []interface{} {
	out := make([]interface{}, 0, len(trace))
	for _, row := range trace {
		entry := map[string]interface{}{"id": row.ID, "evaluated": row.Evaluated, "fired": row.Fired}
		if row.Outcome != "" {
			entry["outcome"] = row.Outcome
		}
		if row.Reason != "" {
			entry["reason"] = row.Reason
		}
		if row.Weight != 0 {
			entry["weight"] = row.Weight
		}
		out = append(out, entry)
	}
	return out
}

// Decide resolves the rule verdicts collected in the shared context (via
// decision/rule.Fire / .Skip in each rule's state-group) into a single
// outcome under the ruleset's policy, and returns the outcome plus a full
// per-rule trace. It clears the collected verdicts so a re-run in the same
// session starts clean. Callers dispatch on `outcome` with
// services/common/assert.Equals.
func (t *engineTransitions) Decide(p *workflow.WorkerSessionContext, ruleset interface{}) (r domain.FlowStepResult) {
	if p == nil {
		r.Error = fmt.Errorf("decision/engine.Decide needs the worker session context (add `p *workflow.WorkerSessionContext`)")
		r.StatusCode = http.StatusInternalServerError
		return
	}
	rs, err := loadRuleset(ruleset)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}
	if errs, _ := validateRuleset(rs); len(errs) > 0 {
		r.Error = fmt.Errorf("ruleset %q is invalid: %s", rs.Name, strings.Join(errs, "; "))
		r.StatusCode = http.StatusUnprocessableEntity
		return
	}

	verdicts := collectVerdicts(p)
	p.RemoveValue(verdictsKey)

	result, decideErr := resolve(rs, verdicts)
	if decideErr != nil {
		r.Error = decideErr
		r.StatusCode = http.StatusUnprocessableEntity
		return
	}

	response := map[string]interface{}{
		"outcome":  result.Outcome,
		"fired":    result.Fired,
		"outcomes": result.Outcomes,
		"policy":   result.Policy,
		"ruleset":  result.Ruleset,
		"trace":    traceToInterfaces(result.Trace),
		"default":  result.Outcome == rs.DefaultOutcome && len(result.Fired) == 0,
	}
	if rs.Policy == policyScore {
		response["score"] = result.Score
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = response
	return
}

// Explain turns a Decide trace into an ordered, human/audit-readable account
// of which rules fired and how the outcome was reached. Pure formatter.
func (t *engineTransitions) Explain(trace []interface{}, outcome string) (r domain.FlowStepResult) {
	lines := make([]string, 0, len(trace)+1)
	fired := 0
	for _, raw := range trace {
		row, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := row["id"].(string)
		reason, _ := row["reason"].(string)
		didFire, _ := row["fired"].(bool)
		evaluated, _ := row["evaluated"].(bool)

		switch {
		case didFire:
			fired++
			if reason != "" {
				lines = append(lines, fmt.Sprintf("+ %s: %s", id, reason))
			} else {
				lines = append(lines, fmt.Sprintf("+ %s: fired", id))
			}
		case evaluated:
			lines = append(lines, fmt.Sprintf("- %s: skipped", id))
		default:
			lines = append(lines, fmt.Sprintf(". %s: not reached", id))
		}
	}
	lines = append(lines, fmt.Sprintf("=> decision: %s (%d rule(s) fired)", strings.TrimSpace(outcome), fired))

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"lines": lines, "text": strings.Join(lines, "\n")}
	return
}
