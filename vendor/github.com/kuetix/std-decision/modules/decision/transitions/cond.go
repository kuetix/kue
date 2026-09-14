package transitions

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

// condTransitions provides the condition primitives a rule state-group builds
// its `when` logic from. Every method returns r.Success = true/false with NO
// r.Error on the false path, so `on success ->` / `on fail ->` both route
// cleanly (this engine build treats any r.Error as fatal and skips the
// workflow's own `on fail`). Compose AND with WSL state chaining, OR with
// several states pointing at one Fire state.
type condTransitions struct {
	workflow.BaseServiceTransition
}

// NewCondTransitions is the DI constructor for the `decision/cond` class.
func NewCondTransitions() interfaces.ServiceTransitions {
	return &condTransitions{}
}

func condResult(match bool, detail map[string]interface{}) (r domain.FlowStepResult) {
	r.Success = match
	r.StatusCode = http.StatusOK
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["match"] = match
	r.Response = detail
	return
}

// Number compares a numeric fact against a threshold.
// op: gt | gte | lt | lte | eq | ne | between (threshold + upper).
func (t *condTransitions) Number(value float64, op string, threshold float64, upper float64) (r domain.FlowStepResult) {
	match := false
	switch strings.TrimSpace(strings.ToLower(op)) {
	case "gt", ">":
		match = value > threshold
	case "gte", ">=", "ge":
		match = value >= threshold
	case "lt", "<":
		match = value < threshold
	case "lte", "<=", "le":
		match = value <= threshold
	case "eq", "==", "":
		match = value == threshold
	case "ne", "!=", "<>":
		match = value != threshold
	case "between":
		match = value >= threshold && value <= upper
	default:
		r.Error = fmt.Errorf("cond.Number: unknown op %q", op)
		r.StatusCode = http.StatusBadRequest
		return
	}
	return condResult(match, map[string]interface{}{"value": value, "op": op, "threshold": threshold})
}

// Text compares a string fact.
// op: eq | ne | contains | prefix | suffix | matches (RE2 regex) | empty | notEmpty.
func (t *condTransitions) Text(value string, op string, other string) (r domain.FlowStepResult) {
	v := value
	o := other
	match := false
	switch strings.TrimSpace(strings.ToLower(op)) {
	case "eq", "==", "":
		match = v == o
	case "ne", "!=":
		match = v != o
	case "contains":
		match = strings.Contains(v, o)
	case "prefix", "startswith":
		match = strings.HasPrefix(v, o)
	case "suffix", "endswith":
		match = strings.HasSuffix(v, o)
	case "matches", "regex":
		re, err := regexp.Compile(o)
		if err != nil {
			r.Error = fmt.Errorf("cond.Text: invalid regex %q: %w", o, err)
			r.StatusCode = http.StatusBadRequest
			return
		}
		match = re.MatchString(v)
	case "empty":
		match = strings.TrimSpace(v) == ""
	case "notempty":
		match = strings.TrimSpace(v) != ""
	default:
		r.Error = fmt.Errorf("cond.Text: unknown op %q", op)
		r.StatusCode = http.StatusBadRequest
		return
	}
	return condResult(match, map[string]interface{}{"value": value, "op": op, "other": other})
}

// In reports whether a string fact is one of the given set. Pass negate=true
// for "not in".
func (t *condTransitions) In(value string, set []interface{}, negate bool) (r domain.FlowStepResult) {
	found := false
	for _, item := range set {
		if fmt.Sprintf("%v", item) == value {
			found = true
			break
		}
	}
	if negate {
		found = !found
	}
	return condResult(found, map[string]interface{}{"value": value, "size": len(set), "negate": negate})
}

// Bool passes a boolean fact straight through as the match verdict.
func (t *condTransitions) Bool(value bool) (r domain.FlowStepResult) {
	return condResult(value, nil)
}

// Expr is the escape hatch: evaluate an expr-lang boolean expression against
// the fact map. Sandboxed (no I/O, no injected functions); bounded by the
// data. Use it when a condition is more naturally one expression than a
// chain of cond.* states.
func (t *condTransitions) Expr(expression string, facts map[string]interface{}) (r domain.FlowStepResult) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		r.Error = fmt.Errorf("cond.Expr: expression is required")
		r.StatusCode = http.StatusBadRequest
		return
	}
	if facts == nil {
		facts = map[string]interface{}{}
	}
	program, err := expr.Compile(expression, expr.AsBool(), expr.AllowUndefinedVariables(), expr.Env(facts))
	if err != nil {
		r.Error = fmt.Errorf("cond.Expr: %w", err)
		r.StatusCode = http.StatusBadRequest
		return
	}
	out, err := expr.Run(program, facts)
	if err != nil {
		r.Error = fmt.Errorf("cond.Expr: %w", err)
		r.StatusCode = http.StatusUnprocessableEntity
		return
	}
	match, ok := out.(bool)
	if !ok {
		r.Error = fmt.Errorf("cond.Expr: expression did not evaluate to a boolean (got %T)", out)
		r.StatusCode = http.StatusUnprocessableEntity
		return
	}
	return condResult(match, map[string]interface{}{"expr": expression})
}
