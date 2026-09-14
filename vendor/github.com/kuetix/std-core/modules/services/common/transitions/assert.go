package transitions

import (
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/helpers"
)

type assertTransitions struct {
	workflow.BaseServiceTransition
}

func NewAssertTransitions() interfaces.ServiceTransitions {
	return &assertTransitions{}
}

//goland:noinspection GoUnusedParameter
func (et *assertTransitions) Bool(value bool) (r domain.FlowStepResult) {
	r.Success = value
	return
}

func (et *assertTransitions) Integer(value int, op string) (r domain.FlowStepResult) {
	var result bool
	key, _ := et.Ctx.ExprLookup(op)
	var test interface{} = key
	var err error
	if _, ok := key.(string); ok {
		_, test, err = et.Ctx.ParseProperty(key.(string) + "|parse|int")
	}
	if err == nil && test != nil {
		result = helpers.AssertInteger(op, value, test.(int))
	}

	r.Success = result
	return
}

func (et *assertTransitions) String(value string, op string) (r domain.FlowStepResult) {
	// var operator []string = []string{"eq", "ne", "switch"}
	var result bool
	if _, test, err := et.Ctx.GetProperty(op + "|parse|string"); err == nil && test != nil {
		result = helpers.AssertString(op, value, test.(string))
	}

	r.Success = result
	return
}

func (et *assertTransitions) Switch(value string) (r domain.FlowStepResult) {
	var operator = []string{"switch", "nin-switch"}
	var result bool
	var ops = make(map[string]bool)
	for _, op := range operator {
		if test, ok := et.Ctx.GetPropertyWithCheck(op); ok {
			r.Next = helpers.AssertSwitch(value, test.(map[string]interface{}))
			if op == "switch" {
				result = r.Next != ""
				if result {
					ops[op] = true
				}
			}
			if op == "nin-switch" {
				result = r.Next == ""
				if result {
					ops[op] = true
				}
			}
		}
	}

	if len(ops) == 1 {
		r.Success = ops["switch"] || ops["nin-switch"]
		return
	}

	if len(ops) == 2 {
		r.Success = ops["switch"] && ops["nin-switch"]
		return
	}

	r.Success = false
	return
}

//goland:noinspection GoUnusedParameter
func (et *assertTransitions) IsPathExists(value map[string]interface{}, path []string) (r domain.FlowStepResult) {
	r.Success = helpers.IsPathExists(value, path)

	return
}

// Equals reports whether value == other. `String(value, op)` above needs
// op to be a property *name* to look up (`Ctx.GetProperty(op +
// "|parse|string")`), not a literal to compare against directly - this is
// the plain, direct "does this string equal that string" a multi-way WSL
// dispatch needs, given `on success when <<expr>> == true -> State` is
// unreliable in this engine (see zmist/backend README's "Known engine
// limitations": it took the "when" branch unconditionally in testing).
// Chain it: `action services/common/assert.Equals(value: $x, other: "a") as
// check; on success -> HandleA; on fail -> NextCheck`.
func (et *assertTransitions) Equals(value string, other string) (r domain.FlowStepResult) {
	r.Success = value == other
	return
}

func (et *assertTransitions) NotEmpty(value string, negative any) (r domain.FlowStepResult) {
	value = strings.Trim(value, " ")
	if value == "" {
		r.Success = false
		r.Response = negative
		return
	}

	r.Success = true
	return
}
