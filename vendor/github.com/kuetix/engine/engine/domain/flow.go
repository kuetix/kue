package domain

import (
	"fmt"
	"strings"

	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/internal/wsl"
	"github.com/kuetix/helpers"
)

const (
	StateInitial = "initial"
	StateNormal  = "normal"
	StateFinal   = "final"
)

var OrderOptionsSearch = []string{"parent.transition", "parent", "transition", "flow"}
var OrderSubOptionsSearch = []string{"constants", "params", "options"}

type FlowState struct {
	Name           string                 `json:"name"`
	State          string                 `json:"state"`
	ContinueOnFail bool                   `json:"continue_on_fail,omitempty"`
	Start          bool                   `json:"start,omitempty" mapstructure:"start,omitempty"`
	Type           string                 `json:"type,omitempty" mapstructure:"type,omitempty"`
	FinalKind      string                 `json:"final_kind,omitempty" mapstructure:"final_kind,omitempty"`
	Description    string                 `json:"description,omitempty"`
	Roles          []string               `json:"roles"`
	Parent         *FlowState             `mapstructure:"parent,omitempty"`
	Params         []string               `mapstructure:"params,omitempty"`
	Response       string                 `mapstructure:"response,omitempty"`
	Node           *wsl.Node              `mapstructure:"node,omitempty"`
	Options        map[string]interface{} `json:"-,omitempty" mapstructure:",remain"`
}

type FlowTransition struct {
	Name           string                 `json:"name"`
	If             *string                `json:"if,omitempty"`
	Else           *string                `json:"else,omitempty"`
	OnSuccessWhen  *string                `json:"on_success_when,omitempty"`
	SkipTo         *bool                  `json:"skipTo,omitempty"`
	ParallelCount  int                    `json:"parallel_count,omitempty" mapstructure:"parallel_count,omitempty"`
	WaitJoin       string                 `json:"wait_join,omitempty" mapstructure:"wait_join,omitempty"`
	From           []string               `json:"from,omitempty"`
	To             string                 `json:"to"`
	Error          string                 `json:"error,omitempty"`
	Description    string                 `json:"description,omitempty"`
	Roles          []string               `json:"roles"`
	True           string                 `mapstructure:"true,omitempty"`
	False          string                 `mapstructure:"false,omitempty"`
	Start          bool                   `json:"start,omitempty" mapstructure:"start,omitempty"`
	Type           string                 `json:"type,omitempty" mapstructure:"type,omitempty"`
	FinalKind      string                 `json:"final_kind,omitempty" mapstructure:"final_kind,omitempty"`
	ContinueOnFail bool                   `json:"continue_on_fail,omitempty"`
	Parent         *FlowTransition        `mapstructure:"parent,omitempty"`
	Params         map[string]interface{} `mapstructure:"params,omitempty"`
	Response       string                 `mapstructure:"response,omitempty"`
	Node           *wsl.Node              `mapstructure:"node,omitempty"`
	Options        map[string]interface{} `json:"-,omitempty" mapstructure:",remain"`
}

type FlowStepResult struct {
	Success    bool
	Next       string
	Error      error
	Response   interface{}
	StatusCode int
}

type FlowOptions = map[string]*map[string]interface{}

type Flow struct {
	Name              string                 `json:"name"`
	Type              string                 `json:"type"`
	ConfigName        string                 `json:"configName"`
	States            []*FlowState           `json:"states,omitempty"`
	Transitions       []*FlowTransition      `json:"transitions,omitempty"`
	CurrentState      *FlowState             `json:"currentState"`
	CurrentTransition *FlowTransition        `json:"currentTransition"`
	Resolvers         []string               `json:"resolvers,omitempty"`
	Options           map[string]interface{} `json:"-,omitempty" mapstructure:",remain"`
	Parent            *Flow                  `mapstructure:"parent,omitempty"`
	Properties        *FlowOptions
	Trace             []interfaces.TraceInterface
	LastTrace         interfaces.TraceInterface
}

func (f *Flow) FromMap(record map[string]interface{}) error {
	return helpers.FromMap(f, record)
}

func (f *Flow) AllOptions() *FlowOptions {
	options := GetAllOptions(f)
	f.Properties = &options
	return f.Properties
}

func (f *Flow) AsInt(key string, byDefault ...int) int {
	return GetIntOption(f, key, byDefault...)
}

func (f *Flow) AsString(key string, byDefault ...string) string {
	return GetStringOption(f, key, byDefault...)
}

func (f *Flow) AsIntOf(key string) (int, bool) {
	return GetIntOptionFromMap(*f.Properties, key)
}

//goland:noinspection GoUnusedParameter
func (f *Flow) AsStringOf(key string, byDefault ...string) (string, bool) {
	return GetStringOptionFromMap(*f.Properties, key)
}

func (f *Flow) Exists(key string) bool {
	_, ok := f.AsStringOf(key)
	return ok
}

func (f *Flow) Value(key string) interface{} {
	if (*f.Properties)["value"] == nil {
		return nil
	}

	value, ok := (*(*f.Properties)["value"])[key]
	if ok {
		return value
	}

	return nil
}

func (f *Flow) SetValue(key string, value interface{}) interface{} {
	if (*f.Properties)["value"] == nil {
		o := make(map[string]interface{})
		(*f.Properties)["value"] = &o
	}
	(*(*f.Properties)["value"])[key] = value
	return f
}

func GetStringOptionFromMap(options FlowOptions, key string) (value string, ok bool) {
	for _, group := range options {
		for k, v := range *group {
			if k == key {
				return v.(string), true
			}
		}
	}

	return "", false
}

func GetStringOption(flow *Flow, key string, byDefault ...string) (value string) {
	var optionsMap FlowOptions
	optionsMap["transition"] = &flow.CurrentTransition.Options
	optionsMap["flow"] = &flow.Options

	if flow.Parent != nil {
		optionsMap["parent.transition"] = &flow.Parent.CurrentTransition.Options
	}
	if flow.Parent != nil {
		optionsMap["parent"] = &flow.Parent.Options
	}
	value, ok := GetStringOptionFromMap(optionsMap, key)
	if ok {
		return value
	}

	if len(byDefault) > 0 {
		return byDefault[0]
	}

	return ""

}

func GetIntOptionFromMap(options FlowOptions, key string) (value int, ok bool) {
	for _, group := range options {
		for k, v := range *group {
			if k == key {
				return int(v.(float64)), true
			}
		}
	}

	return 0, false
}

func GetIntOption(flow *Flow, key string, byDefault ...int) (value int) {
	var optionsMap FlowOptions
	optionsMap["transition"] = &flow.CurrentTransition.Options
	optionsMap["flow"] = &flow.Options

	if flow.Parent != nil {
		optionsMap["parent.transition"] = &flow.Parent.CurrentTransition.Options
	}
	if flow.Parent != nil {
		optionsMap["parent"] = &flow.Parent.Options
	}
	value, ok := GetIntOptionFromMap(optionsMap, key)
	if ok {
		return value
	}

	if len(byDefault) > 0 {
		return byDefault[0]
	}

	return 0

}

func GetAllOptions(flow *Flow) (options FlowOptions) {
	options = make(FlowOptions)
	if flow.CurrentTransition.Options != nil {
		if flow.CurrentTransition.Options != nil {
			options["transition"] = &flow.CurrentTransition.Options
		}
	}

	if flow.Options != nil {
		if flow.Options != nil {
			options["flow"] = &flow.Options
		}
	}

	if flow.Parent != nil {
		if flow.Parent.Options != nil {
			options["parent"] = &flow.Parent.Options
		}
	}

	if flow.Parent != nil {
		options["parent.transition"] = GetFlowParentsOptions(flow)
	}

	return options
}

func GetFlowParentsOptions(flow *Flow) *map[string]interface{} {
	var parents []*Flow
	for parent := flow.Parent; parent != nil; parent = parent.Parent {
		parents = append(parents, parent)
	}
	var opts = make(map[string]interface{})
	for i := len(parents) - 1; i >= 0; i-- {
		parent := parents[i]
		if parent.CurrentTransition.Options != nil {
			opts = helpers.MergeMapsLevel0(opts, parent.CurrentTransition.Options)
		}
	}

	return &opts
}

func FlowOptionsPtrKey(m *map[string]interface{}, key string) *FlowOptions {
	if _, ok := (*m)[key]; !ok {
		o := make(FlowOptions)
		(*m)[key] = &o
	}

	return (*m)[key].(*FlowOptions)
}

func FlowOptionsKey(options *FlowOptions, key string, order []string, subOrder []string) (interface{}, bool) {
	if !strings.HasPrefix(key, "<<") && strings.Contains(key, ".") {
		keys := strings.Split(key, ".")
		group := strings.Join(keys[:len(keys)-1], ".")
		key = keys[len(keys)-1]
		if len(keys) > 1 {
			if grpOpts, ok := (*options)[group]; ok {
				if i, ok := (*grpOpts)[key]; ok {
					return i, ok
				}
			}
		}
	}

	var ok bool
	var grpOpts *map[string]interface{}
	var i interface{}
	if len(order) > 0 {
		for _, group := range order {
			if grpOpts, ok = (*options)[group]; ok {
				if i, ok = (*grpOpts)[key]; ok {
					return i, ok
				}
				if len(subOrder) > 0 {
					for _, subGroup := range subOrder {
						if i, ok = (*grpOpts)[subGroup]; ok {
							if i, ok = i.(map[string]interface{})[key]; ok {
								return i, ok
							}
						}
					}
				}
			}
		}
	}

	return nil, false
}

func (f *Flow) GetTrace() map[string]interface{} {
	if f.Trace == nil {
		return map[string]interface{}{}
	}
	var ts []string
	for _, value := range f.Trace {
		ts = append(ts, fmt.Sprintf("%s -> %s", value.GetFrom(), value.GetTo()))
	}

	options, _ := helpers.ToMapRecursive(f.AllOptions())
	if _, ok := options["flow"]; ok {
		if _, ok = options["flow"].(map[string]interface{})["ast"]; ok {
			delete(options["flow"].(map[string]interface{}), "ast")
		}
	}
	return map[string]interface{}{
		"[1]name":    f.Name,
		"[2]trace":   ts,
		"[3]options": options,
	}
}

func (f *Flow) GetTraceString() string {
	return fmt.Sprintf("%+v", f.GetTrace())
}
