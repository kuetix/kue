package transitions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type factsTransitions struct {
	workflow.BaseServiceTransition
}

// NewFactsTransitions is the DI constructor for the `decision/facts` class.
func NewFactsTransitions() interfaces.ServiceTransitions {
	return &factsTransitions{}
}

// prepareFacts is the shared normaliser: it applies the ruleset's `inputs`
// schema to a raw fact map - filling declared defaults, coercing declared
// types, and reporting missing required facts. Facts not named in the
// schema are passed through untouched.
func prepareFacts(rs Ruleset, raw map[string]interface{}) (prepared map[string]interface{}, applied []string, coerced []string, missing []string) {
	prepared = map[string]interface{}{}
	for k, v := range raw {
		prepared[k] = v
	}

	names := make([]string, 0, len(rs.Inputs))
	for name := range rs.Inputs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		spec := rs.Inputs[name]
		value, present := prepared[name]

		if !present || value == nil {
			if spec.Default != nil {
				casted, err := castValue(spec.Default, spec.Type)
				if err != nil {
					casted = spec.Default
				}
				prepared[name] = casted
				applied = append(applied, name)
				continue
			}
			if spec.Required {
				missing = append(missing, name)
			}
			continue
		}

		casted, err := castValue(value, spec.Type)
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s (not a %s)", name, spec.Type))
			continue
		}
		if !sameValue(casted, value) {
			coerced = append(coerced, name)
		}
		prepared[name] = casted
	}
	return prepared, applied, coerced, missing
}

func sameValue(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) && fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b)
}

// asFactMap coerces whatever the workflow passed for `facts` (a real map, a
// JSON string, or nothing at all when the arg reference did not resolve) into
// a fact map. A missing/empty value is not an error here - the ruleset's
// required-field check reports it cleanly below.
func asFactMap(raw interface{}) (map[string]interface{}, error) {
	switch v := raw.(type) {
	case nil:
		return map[string]interface{}{}, nil
	case map[string]interface{}:
		return v, nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" || s == "<nil>" || strings.HasPrefix(s, "<<") {
			return map[string]interface{}{}, nil
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return nil, fmt.Errorf("facts is a string but not a JSON object: %w", err)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("facts must be an object or a JSON string (got %T)", raw)
	}
}

// Prepare normalises an incoming fact set against a ruleset's `inputs`
// schema (defaults, type coercion, required checks) so the downstream
// `cond.*` checks compare like-typed values. It fails when a required fact is
// missing or a supplied fact cannot be coerced to its declared type.
func (t *factsTransitions) Prepare(ruleset interface{}, facts interface{}) (r domain.FlowStepResult) {
	rs, err := loadRuleset(ruleset)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}
	factMap, err := asFactMap(facts)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	prepared, applied, coerced, missing := prepareFacts(rs, factMap)
	if len(missing) > 0 {
		r.Error = fmt.Errorf("facts do not satisfy ruleset %q: missing/invalid %s", rs.Name, strings.Join(missing, ", "))
		r.StatusCode = http.StatusBadRequest
		r.Response = map[string]interface{}{"missing": missing}
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"facts":           prepared,
		"appliedDefaults": applied,
		"coerced":         coerced,
	}
	return
}
