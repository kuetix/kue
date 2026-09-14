package workflow

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/domain/issues"
	h "github.com/kuetix/engine/engine/helpers"
	"github.com/kuetix/engine/internal/wsl"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

const LimitTrace = 20

type EngineInterface interface {
	LoadWorkflow(c context.Context, configName string, worker Worker) bool
	Start() bool
	Run() bool
	Done() *WorkerResponse
	Process(c context.Context, workflowName string, worker Worker) map[string]*WorkerResponse
	ApplicationContext() context.Context
	SetModulesPath(path string)
	SetWorkflowName(workflowName string)
	LoadWorkflowByPath(name string) error
	GetResolvers() []string
	GetFlow() *domain.Flow
	SetPreventLogError(state bool)
	isPreventedLogError() bool
	GetWorker() *Worker
	GetEngineName() string
	GetModulesDir() string
	GetApplication() domain.Application
	GetWorkflowConfig() domain.WorkflowConfigItem
	LoadWorkflowByName(name string) (map[string]interface{}, error)
	GetWorkflowFilePath(name string) (r FilePathResult, err error)
	WSLLoadToSchema(name string, filePath string, withPrefix bool) (map[string]interface{}, error)
	SWSLLoadToSchema(name string, filePath string, withPrefix bool) (map[string]interface{}, error)
	JsonLoadToSchema(name string, err error, filePath string) (map[string]interface{}, error)
	CorrectFlow(flow *domain.Flow) error
	EnsureNextState(name string)
	GetWorkflowActions(workflowName string) ([]WorkflowAction, error)
	GetAllWorkflowActions() map[string][]WorkflowAction
}

// WorkflowAction describes a single service action referenced by a WSL
// workflow state. It surfaces the action's module/method and the raw
// arguments (as written in WSL) along with parsed named-argument names.
type WorkflowAction struct {
	Workflow string        `json:"workflow"`
	State    string        `json:"state"`
	Module   string        `json:"module,omitempty"`
	Name     string        `json:"name"`
	As       string        `json:"as,omitempty"`
	Args     []WorkflowArg `json:"args,omitempty"`
	ArgNames []string      `json:"argNames,omitempty"`
	Params   []string      `json:"params,omitempty"`
	Terminal string        `json:"terminal,omitempty"`
}

// WorkflowArg is one named argument in a workflow action call.
type WorkflowArg struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value"`
	Raw   string `json:"raw"`
}

type Engine struct {
	Name            string
	WorkflowName    string
	WorkingDir      string
	ModulesPath     string
	WorkflowPath    string
	Flow            *domain.Flow
	Flows           map[string]*domain.Flow
	Engine          EngineInterface
	Worker          Worker
	AppContext      context.Context
	PreventLogError bool
	WorkflowsPaths  map[string]string
	Application     domain.Application
	WorkflowConfig  domain.WorkflowConfigItem
}

type FilePathResult struct {
	OriginalName    string
	Name            string
	FilePath        string
	FileExt         string
	FileBase        string
	Basedir         string
	IsJson          bool
	IsWSL           bool
	IsSimplifiedWSL bool
	WithPrefix      bool
}

func NewWorkflowEngine(engineName string, app domain.Application) EngineInterface {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	modulesPath, _ := h.ModulesPath(app.Env.Config.Application.ModulesPath)
	workflowsPath, _ := h.ModulesPath(app.Env.Config.Application.WorkflowsPath)
	engine := &Engine{
		Name:           engineName,
		ModulesPath:    modulesPath,
		WorkflowPath:   workflowsPath,
		Flow:           nil,
		WorkingDir:     wd,
		Engine:         nil,
		Worker:         nil,
		AppContext:     nil,
		WorkflowsPaths: make(map[string]string),
		Application:    app,
	}
	engine.Engine = engine

	return engine
}

func (w *Engine) SetModulesPath(path string) {
	w.ModulesPath = path
}

func (w *Engine) SetWorkflowName(workflowName string) {
	w.WorkflowName = workflowName
}

func (w *Engine) LoadWorkflowByPath(workflowName string) error {
	w.Flow = &domain.Flow{}
	schemas, err := w.LoadWorkflowByName(workflowName)
	if err != nil {
		return err
	}
	w.Flows = make(map[string]*domain.Flow)
	for name, schema := range schemas {
		flowType := w.extractFlowType(schema)
		w.Flows[name] = &domain.Flow{Name: name, Type: flowType, ConfigName: workflowName, Properties: &domain.FlowOptions{}}
		err = w.Flows[name].FromMap(schema.(map[string]interface{}))
		if err != nil {
			return errors.New(fmt.Sprintf("In %s: %s: Can't load %s json with message: %s", w.Name, w.WorkflowName, name, err.Error()))
		}

		err = w.CorrectFlow(w.Flows[name])
		if err != nil {
			return err
		}
	}

	return nil
}

// extractFlowType extracts the flow type from the schema, defaulting to "workflow"
func (w *Engine) extractFlowType(schema interface{}) string {
	schemaMap, ok := schema.(map[string]interface{})
	if !ok {
		return "workflow"
	}

	typeVal, hasType := schemaMap["type"]
	if !hasType {
		return "workflow"
	}

	typeStr, isStr := typeVal.(string)
	if !isStr {
		return "workflow"
	}

	return typeStr
}

func (w *Engine) GetResolvers() []string {
	return w.Flow.Resolvers
}

func (w *Engine) LoadWorkflowByName(name string) (map[string]interface{}, error) {
	f, err := w.GetWorkflowFilePath(name)
	if err != nil {
		return nil, err
	}

	name = f.Name

	var schemas map[string]interface{}

	if f.IsJson {
		var schema map[string]interface{}
		schema, err = w.JsonLoadToSchema(name, err, f.FilePath)
		if err == nil {
			schemas = map[string]interface{}{
				name: schema,
			}
		}
	} else if f.IsWSL {
		schemas, err = w.WSLLoadToSchema(name, f.FilePath, f.WithPrefix)
	} else if f.IsSimplifiedWSL {
		schemas, err = w.SWSLLoadToSchema(name, f.FilePath, f.WithPrefix)
	}

	if err != nil {
		return nil, err
	}

	return schemas, nil
}

func (w *Engine) GetWorkflowFilePath(name string) (r FilePathResult, err error) {
	logger.Debugf("In %s: %s: GetWorkflowFilePath: %s", w.Name, w.WorkflowName, name)
	r = FilePathResult{
		OriginalName:    name,
		Name:            name,
		FilePath:        "",
		FileExt:         "",
		FileBase:        "",
		Basedir:         "",
		IsJson:          false,
		IsWSL:           false,
		IsSimplifiedWSL: false,
		WithPrefix:      false,
	}

	found := false
	if strings.HasPrefix(name, "@") {
		r.WithPrefix = true
		name = name[1:]
		r.Name = name
		names := strings.Split(name, "/")
		names = append(names[:1], append([]string{"workflow"}, names[1:]...)...)
		namePath := strings.Join(names, "/")

		nameWOExt := strings.TrimSuffix(name, filepath.Ext(name))

		filesPaths := []string{
			fmt.Sprintf("%s/%s", w.Application.EmbedFSRootPath, name),
			fmt.Sprintf("%s/%s", w.WorkflowPath, name),
			fmt.Sprintf("%s", name),
			fmt.Sprintf("%s/%s.wsl", w.Application.EmbedFSRootPath, name),
			fmt.Sprintf("%s/%s.wsl", w.WorkflowPath, name),
			fmt.Sprintf("%s.wsl", name),
			fmt.Sprintf("%s/%s.swsl", w.Application.EmbedFSRootPath, name),
			fmt.Sprintf("%s/%s.swsl", w.WorkflowPath, name),
			fmt.Sprintf("%s.swsl", name),
			fmt.Sprintf("%s/%s.json", w.Application.EmbedFSRootPath, name),
			fmt.Sprintf("%s/%s.json", w.WorkflowPath, name),
			fmt.Sprintf("%s.json", name),
			fmt.Sprintf("%s", nameWOExt),
			namePath,
			fmt.Sprintf("%s.wsl", name),
			fmt.Sprintf("%s.wsl", nameWOExt),
			fmt.Sprintf("%s/%s.wsl", w.WorkflowPath, name),
			fmt.Sprintf("%s.swsl", name),
			fmt.Sprintf("%s.swsl", nameWOExt),
			fmt.Sprintf("%s/%s.swsl", w.WorkflowPath, name),
			fmt.Sprintf("%s.json", name),
			fmt.Sprintf("%s.json", nameWOExt),
			fmt.Sprintf("%s/%s.json", w.WorkflowPath, name),
		}

		for _, nameFilePath := range filesPaths {
			// Try to find JSON first, then WSL
			r.FilePath = ""
			nfPathWithExt := nameFilePath
			candidateWSL := strings.ReplaceAll(fmt.Sprintf("%s/%s/%s", w.WorkingDir, w.WorkflowPath, nfPathWithExt), "//", "/")
			if CheckFileExistsInEmbedFS(w.Application.EmbedFS, nfPathWithExt) {
				r.FilePath = nfPathWithExt
				found = true
				break
			}
			if CheckFileExistsInEmbedFS(w.Application.EmbedFS, candidateWSL) {
				r.FilePath = candidateWSL
				found = true
				break
			}
			if _, statErr := os.Stat(candidateWSL); statErr == nil {
				r.FilePath = candidateWSL
				found = true
				break
			}
		}
	} else {
		r.FilePath = strings.ReplaceAll(fmt.Sprintf("%s/%s", w.WorkingDir, name), "//", "/")
		candidates := []string{
			fmt.Sprintf("%s/%s", w.Application.EmbedFSRootPath, name),
			fmt.Sprintf("%s", name),
			fmt.Sprintf("%s/%s.wsl", w.Application.EmbedFSRootPath, name),
			fmt.Sprintf("%s.wsl", name),
			fmt.Sprintf("%s/%s.swsl", w.Application.EmbedFSRootPath, name),
			fmt.Sprintf("%s.swsl", name),
			fmt.Sprintf("%s/%s.json", w.Application.EmbedFSRootPath, name),
			fmt.Sprintf("%s.json", name),
			fmt.Sprintf("%s", r.FilePath),
			fmt.Sprintf("%s.wsl", r.FilePath),
			fmt.Sprintf("%s.swsl", r.FilePath),
			fmt.Sprintf("%s.json", r.FilePath),
		}
		for _, candidate := range candidates {
			if CheckFileExistsInEmbedFS(w.Application.EmbedFS, candidate) {
				r.FilePath = candidate
				found = true
				break
			}
			if _, statErr := os.Stat(candidate); statErr == nil {
				r.FilePath = candidate
				found = true
				break
			}
		}
	}

	if found == false {
		return r, errors.New(fmt.Sprintf("In %s: %s: workflow %s not found", w.Name, w.WorkflowName, name))
	}

	r.FileExt = filepath.Ext(r.FilePath)
	r.FileBase = filepath.Base(r.FilePath)
	r.Basedir = filepath.Dir(r.FilePath)
	switch strings.ToLower(r.FileExt) {
	case ".wsl":
		r.IsWSL = true
	case ".swsl":
		r.IsSimplifiedWSL = true
	case ".json":
		r.IsJson = true
	default:
		return r, errors.New(fmt.Sprintf("In %s: %s: Unsupported file extension for workflow %s: %s", w.Name, w.WorkflowName, name, r.FileExt))
	}

	if r.FilePath == "" {
		return r, errors.New(fmt.Sprintf("In %s: %s: workflow %s not found (neither .json nor .wsl)", w.Name, w.WorkflowName, name))
	}

	return r, nil
}

func (w *Engine) WSLLoadToSchema(name string, filePath string, withPrefix bool) (map[string]interface{}, error) {
	var err error
	// WSL path
	var file []byte
	if CheckFileExistsInEmbedFS(w.Application.EmbedFS, filePath) {
		file, err = w.Application.EmbedFS.ReadFile(filePath)
	} else {
		file, err = os.ReadFile(filePath)
	}
	if err != nil {
		return nil, errors.New(fmt.Sprintf("In %s: %s: Can't load %s wsl with message: %s", w.Name, w.WorkflowName, name, err.Error()))
	}
	var ast *wsl.Module
	var graphs map[string]*wsl.Graph
	ast, graphs, err = wsl.ParseAll(string(file), name)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("In %s: %s: WSL parse error for %s: %s", w.Name, w.WorkflowName, name, err.Error()))
	}
	if len(graphs) < 0 {
		return nil, errors.New(fmt.Sprintf("In %s: %s: WSL workflow not found in module %s", w.Name, w.WorkflowName, ast.Name))
	}

	var schemas = make(map[string]interface{})
	schemas, err = w.buildSchemaMap(name, graphs, withPrefix, ast)
	if err != nil {
		return nil, err
	}

	var schema map[string]interface{}
	var resolvers []string
	var constants = make(map[string]interface{})
	for _, c := range ast.Constants {
		constants[c.Name] = c.Value
	}
	var want string
	var graph interface{}
	for want, graph = range schemas {
		if graphType, ok := graph.(map[string]interface{})["type"]; ok && graphType.(string) != "workflow" {
			continue
		}
		schema = graph.(map[string]interface{})
		rs := schema["resolvers"]
		if rs != nil {
			if helpers.IsSlice(rs) {
				if len(rs.([]string)) > 0 {
					resolvers = helpers.AppendUnique(resolvers, rs.([]string))
				}
			} else {
				if _, ok := rs.(string); ok {
					resolvers = helpers.AppendStringUnique(resolvers, rs.(string))
				}
				resolvers = append(resolvers, rs.(string))
			}
		} else {
			logger.Warnf("In %s: %s: WSL workflow %s has no resolvers defined", w.Name, w.WorkflowName, want)
		}
		if ts, ok := schema["transitions"]; ok {
			for n := range ts.([]map[string]interface{}) {
				t := ts.([]map[string]interface{})[n]
				if cs, ok := t["constants"]; ok {
					if cs != nil {
						constants = helpers.MergeMaps(constants, cs.(map[string]interface{}))
					} else {
						logger.Warnf("In %s: %s: WSL workflow %s has no resolvers defined", w.Name, w.WorkflowName, want)
					}
				}
				schema["transitions"].([]map[string]interface{})[n]["constants"] = constants
			}
		}
	}

	schemas = w.updateSchemas(schemas, resolvers, constants)

	return schemas, nil
}

func (w *Engine) SWSLLoadToSchema(name string, filePath string, withPrefix bool) (map[string]interface{}, error) {
	var err error
	// WSL path
	var file []byte
	if CheckFileExistsInEmbedFS(w.Application.EmbedFS, filePath) {
		file, err = w.Application.EmbedFS.ReadFile(filePath)
	} else {
		file, err = os.ReadFile(filePath)
	}
	if err != nil {
		return nil, errors.New(fmt.Sprintf("In %s: %s: Can't load %s swsl with message: %s", w.Name, w.WorkflowName, name, err.Error()))
	}
	var ast *wsl.Module
	var graphs map[string]*wsl.Graph
	ast, graphs, err = wsl.ParseAllSimplified(string(file), name)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("In %s: %s: SWSL parse error for %s: %s", w.Name, w.WorkflowName, name, err.Error()))
	}
	if len(graphs) < 0 {
		return nil, errors.New(fmt.Sprintf("In %s: %s: SWSL workflow not found in module %s", w.Name, w.WorkflowName, ast.Name))
	}

	var schemas = make(map[string]interface{})
	schemas, err = w.buildSchemaMap(name, graphs, withPrefix, ast)
	if err != nil {
		return nil, err
	}

	var schema map[string]interface{}
	var resolvers []string
	var constants = make(map[string]interface{})
	for _, c := range ast.Constants {
		constants[c.Name] = c.Value
	}
	var want string
	var graph interface{}
	for want, graph = range schemas {
		if graphType, ok := graph.(map[string]interface{})["type"]; ok && graphType.(string) != "workflow" {
			continue
		}
		schema = graph.(map[string]interface{})
		rs := schema["resolvers"]
		if rs != nil {
			if helpers.IsSlice(rs) && len(rs.([]string)) > 0 {
				resolvers = helpers.AppendUnique(resolvers, rs.([]string))
			} else {
				if _, ok := rs.(string); ok {
					resolvers = helpers.AppendStringUnique(resolvers, rs.(string))
				}
				resolvers = append(resolvers, rs.(string))
			}
		} else {
			logger.Warnf("In %s: %s: SWSL workflow %s has no resolvers defined", w.Name, w.WorkflowName, want)
		}
		if ts, ok := schema["transitions"]; ok {
			for n := range ts.([]map[string]interface{}) {
				t := ts.([]map[string]interface{})[n]
				if cs, ok := t["constants"]; ok {
					if cs != nil {
						constants = helpers.MergeMaps(constants, cs.(map[string]interface{}))
					} else {
						logger.Warnf("In %s: %s: SWSL workflow %s has no resolvers defined", w.Name, w.WorkflowName, want)
					}
				}
				schema["transitions"].([]map[string]interface{})[n]["constants"] = constants
			}
		}
	}

	schemas = w.updateSchemas(schemas, resolvers, constants)

	return schemas, nil
}

func (w *Engine) JsonLoadToSchema(name string, err error, filePath string) (map[string]interface{}, error) {
	var file []byte
	if CheckFileExistsInEmbedFS(w.Application.EmbedFS, filePath) {
		file, err = w.Application.EmbedFS.ReadFile(filePath)
	} else {
		file, err = os.ReadFile(filePath)
	}
	if err != nil {
		return nil, errors.New(fmt.Sprintf("In %s: %s: Can't load %s json with message: %s", w.Name, w.WorkflowName, name, err.Error()))
	}

	schema := make(map[string]interface{})
	err = json.Unmarshal(file, &schema)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("In %s/%s.json: Can't load json with message: %s", w.ModulesPath, name, err.Error()))
	}

	if extendsName, ok := schema["extends"]; ok {
		delete(schema, "extends")
		extends, err := w.LoadWorkflowByName(extendsName.(string))
		if err != nil {
			return nil, err
		}
		schema = helpers.MergeMaps(schema, extends)
	}

	if includesName, ok := schema["include"]; ok {
		delete(schema, "include")
		var i []interface{}
		if helpers.IsSlice(includesName) {
			i = includesName.([]interface{})
		} else {
			i = []interface{}{includesName}
		}
		for _, includeName := range i {
			include, err := w.LoadWorkflowByName(includeName.(string))
			if err != nil {
				return nil, err
			}
			schema = helpers.MergeMaps(schema, include)
		}
	}

	return schema, nil
}

func resolveUnderscoreBranchTarget(transitions []*domain.FlowTransition, transition *domain.FlowTransition, idx int, branchName string, target string) (string, error) {
	if target != "_" {
		return target, nil
	}
	if idx == len(transitions)-1 {
		// If trailing success path uses "_", treat current transition as final success.
		if branchName == "true" {
			transition.Type = domain.StateFinal
			if transition.FinalKind == "" {
				transition.FinalKind = "ok"
			}
			return "", nil
		}
		return "", fmt.Errorf("transition '%s': '_' in %s branch requires a following transition or explicit target", transition.Name, branchName)
	}
	return transitions[idx+1].To, nil
}

func (w *Engine) CorrectFlow(flow *domain.Flow) error {
	var lastTransition *domain.FlowTransition
	var statesExists = make(map[string]bool, len(flow.Transitions))
	var statesExistsSorted = make([]string, len(statesExists))
	var duplicatesCount = make(map[string]int)
	var statesTypes = make(map[string]string)
	for i, transition := range flow.Transitions {
		state := transition.To
		duplicatesCount[state]++
		if strings.ContainsRune(state, '#') == false || duplicatesCount[state] > 1 {
			state = fmt.Sprintf("%s#%d", state, duplicatesCount[state])
		}
		transition.To = state
		if transition.From == nil {
			if lastTransition != nil {
				transition.From = append(transition.From, lastTransition.To)
			} else {
				transition.From = []string{"_"}
			}
		}
		for i, from := range transition.From {
			if from != "_" && strings.ContainsRune(from, '#') == false {
				transition.From[i] = fmt.Sprintf("%s#%d", from, 1)
			}
		}
		if transition.True != "" {
			resolved, err := resolveUnderscoreBranchTarget(flow.Transitions, transition, i, "true", transition.True)
			if err != nil {
				return err
			}
			transition.True = resolved
			if transition.True != "" && strings.ContainsRune(transition.True, '#') == false {
				transition.True = fmt.Sprintf("%s#%d", transition.True, 1)
			}
		}
		if transition.False != "" {
			resolved, err := resolveUnderscoreBranchTarget(flow.Transitions, transition, i, "false", transition.False)
			if err != nil {
				return err
			}
			transition.False = resolved
			if strings.ContainsRune(transition.False, '#') == false {
				transition.False = fmt.Sprintf("%s#%d", transition.False, 1)
			}
		}
		if transition.Else != nil && *transition.Else != "" {
			resolved, err := resolveUnderscoreBranchTarget(flow.Transitions, transition, i, "else", *transition.Else)
			if err != nil {
				return err
			}
			*transition.Else = resolved
			if strings.ContainsRune(*transition.Else, '#') == false {
				*transition.Else = fmt.Sprintf("%s#%d", *transition.Else, 1)
			}
		}
		lastTransition = transition
		statesExists[transition.To] = false
		statesExistsSorted = append(statesExistsSorted, transition.To)
		statesTypes[transition.To] = transition.Type
	}
	var initial = false
	var final = false
	for _, state := range flow.States {
		if state.Type == domain.StateInitial {
			initial = true
		}
		if state.Type == domain.StateFinal {
			final = true
		}
		if _, exists := statesExists[state.State]; exists {
			statesExists[state.State] = true
		}
	}
	var count = 0
	var last = len(statesExists) - 1
	for _, state := range statesExistsSorted {
		exists := statesExists[state]
		if exists == false {
			var typeState = domain.StateNormal
			if count == 0 && !initial {
				typeState = domain.StateInitial
			}
			if count == last && !final {
				typeState = domain.StateFinal
			}
			stateType := statesTypes[state]
			if stateType != "" {
				typeState = stateType
			}
			flow.States = append(flow.States, &domain.FlowState{
				State:          state,
				Type:           typeState,
				ContinueOnFail: false,
			})
		}
		count++
	}

	return nil
}

func (w *Engine) getNextTransition(transitions []*domain.FlowTransition, current *domain.FlowState) *domain.FlowTransition {
	if current == nil {
		for _, transition := range transitions {
			if helpers.FindStringIndex(transition.From, "_") > -1 {
				return transition
			}
		}

		return nil
	}
	for _, transition := range transitions {
		if helpers.FindStringIndex(transition.From, current.State) > -1 {
			return transition
		}
	}

	return nil
}

func (w *Engine) getNextTransitionWithNext(transitions []*domain.FlowTransition, current *domain.FlowState, next *domain.FlowState) *domain.FlowTransition {
	if current == nil {
		for _, transition := range transitions {
			if helpers.FindStringIndex(transition.From, "_") > -1 {
				return transition
			}
		}

		return nil
	}
	for _, transition := range transitions {
		if helpers.FindStringIndex(transition.From, current.State) > -1 && next != nil && transition.To == next.State {
			return transition
		}
	}

	return nil
}

func (w *Engine) getTransitionState(states []*domain.FlowState, transition *domain.FlowTransition) *domain.FlowState {
	if transition == nil {
		return nil
	}
	hash := strings.SplitN(transition.To, "#", 2)
	toHash := hash[1]
	toStateName := hash[0]
	for _, state := range states {
		hashState := strings.SplitN(state.State, "#", 2)
		stateName := hashState[0]
		var stateHash string
		if len(hashState) < 2 {
			stateHash = "1"
		} else {
			stateHash = hashState[1]
		}
		if toStateName == stateName && toHash == stateHash {
			result := &domain.FlowState{
				State:          state.State,
				ContinueOnFail: state.ContinueOnFail,
				Type:           state.Type,
			}

			if len(hash) > 1 {
				result.State = transition.To
			}

			return result
		}
	}

	return nil
}

func (w *Engine) getState(states []*domain.FlowState, stateName string) *domain.FlowState {
	hash := strings.SplitN(stateName, "#", 2)
	findStateName := hash[0]
	var findStateHash string
	if len(hash) > 1 {
		findStateHash = hash[1]
	} else {
		findStateHash = "1"
	}
	for _, state := range states {
		hashState := strings.SplitN(state.State, "#", 2)
		thisStateName := hashState[0]
		var stateHash string
		if len(hashState) < 2 {
			stateHash = "1"
		} else {
			stateHash = hashState[1]
		}
		if findStateName == thisStateName && findStateHash == stateHash {
			result := &domain.FlowState{
				State:          state.State,
				ContinueOnFail: state.ContinueOnFail,
				Type:           state.Type,
			}

			if len(hash) > 1 {
				result.State = stateName
			}

			return result
		}
	}

	return nil
}

func (w *Engine) can(customNextStepName string) bool {
	var currentState = w.Flow.CurrentState
	if customNextStepName != "" {
		nextState := w.getState(w.Flow.States, customNextStepName)
		if nextState != nil && nextState.Type == domain.StateFinal && currentState != nil && currentState.State == nextState.State {
			nextTransition := w.getNextTransitionWithNext(w.Flow.Transitions, w.Flow.CurrentState, currentState)
			if nextTransition != nil && w.Flow.CurrentTransition.False != "" && w.Flow.CurrentTransition.False == currentState.State {
				return true
			}
			w.Flow.CurrentState = currentState
			return false
		}
	}
	// If the current state is already terminal and no custom next step was provided,
	// the workflow has completed successfully (handles terminal states with error-only edges).
	if customNextStepName == "" && currentState != nil && currentState.Type == domain.StateFinal {
		return false
	}
	nextTransition := w.getNextTransition(w.Flow.Transitions, w.Flow.CurrentState)

	if nextTransition != nil {
		nextState := w.getTransitionState(w.Flow.States, nextTransition)
		if nextState == nil {
			errMsg := fmt.Sprintf("In %s: %s: From %s -> %s was stopped, nextState not found", w.Name, w.WorkflowName, nextTransition.From, nextTransition.To)
			w.Worker.SetError(issues.NewIssue(errMsg, errors.New(errMsg)))
		}
		return nextState != nil
	}

	if currentState == nil {
		errMsg := fmt.Sprintf("In %s: %s: Current state is nil", w.Name, w.WorkflowName)
		w.Worker.SetError(issues.NewIssue(errMsg, errors.New(errMsg)))

		return false
	}

	if currentState.Type != domain.StateFinal {
		errMsg := fmt.Sprintf("In %s: %s: Current state %s stopped and state is not final", w.Name, w.WorkflowName, currentState.State)
		w.Worker.SetError(issues.NewIssue(errMsg, errors.New(errMsg)))
	}

	return false
}

func (w *Engine) next(customNextStepName string) *domain.Flow {
	var currentState = w.Flow.CurrentState
	var nextTransition *domain.FlowTransition
	if customNextStepName != "" {
		currentState = w.getState(w.Flow.States, customNextStepName)
		nextTransition = w.getNextTransitionWithNext(w.Flow.Transitions, w.Flow.CurrentState, currentState)
	} else {
		nextTransition = w.getNextTransition(w.Flow.Transitions, currentState)
	}
	w.Flow.CurrentTransition = nextTransition
	nextState := w.getTransitionState(w.Flow.States, w.Flow.CurrentTransition)
	w.Flow.CurrentState = nextState

	return w.Flow
}

func (w *Engine) LoadWorkflow(c context.Context, workflowName string, workflowWorker Worker) bool {
	w.WorkflowName = workflowName
	w.AppContext = c
	w.Worker = workflowWorker
	err := w.LoadWorkflowByPath(workflowName)
	if err != nil {
		w.Worker.SetError(issues.NewIssue(fmt.Sprintf("In %s: %s: Can't load workflow json, with message: %s", w.Name, w.WorkflowName, err.Error()), err), http.StatusNotFound)
		return false
	}

	return true
}

func (w *Engine) Start() bool {
	return w.Worker.Start(w.Engine)
}

func (w *Engine) Run() bool {
	var customNextStepName = ""
	w.Worker.PrepareContext(w.Engine, w.Flow)
	var previous = "_"
	for w.can(customNextStepName) {
		w.next(customNextStepName)
		customNextStepName = ""

		// Trace
		if (*w.Flow).CurrentTransition != nil {
			to := (*w.Flow).CurrentTransition.To

			lenTrace := len(w.Flow.Trace) - 1
			var previousTrace interfaces.TraceInterface
			if lenTrace > 0 {
				previousTrace = w.Flow.Trace[lenTrace]
			} else {
				previousTrace = nil
			}
			if w.Worker.IsDebug() {
				trace := domain.NewDebugTrace(previousTrace, previous, to, w.Worker.GetWorkflowContext())
				w.Flow.Trace = append(w.Flow.Trace, trace)
				w.Flow.LastTrace = trace
			} else {
				trace := domain.NewTrace(previousTrace, previous, to, w.Worker.GetWorkflowContext())
				w.Flow.Trace = append(w.Flow.Trace, trace)
				w.Flow.LastTrace = trace
			}
			if len(w.Flow.Trace) > LimitTrace {
				w.Flow.Trace = w.Flow.Trace[1:]
			}
			previous = (*w.Flow).CurrentTransition.To
		} else {
			panic(fmt.Sprintf("CurrentTransition is nil, ensure NEXT transition is correctly named and FROM is supported %s transtition, trace: %v", w.Flow.LastTrace, w.Flow.Trace))
		}

		ok, nextStateName := w.Worker.ProcessState(w.Engine, w.Flow)
		if !ok {
			if w.Flow.CurrentTransition.ContinueOnFail {
				ok = true
			}
		}
		if !ok {
			w.Worker.ProcessStateError(w.Engine, w.Flow, w.Worker.GetWorkerResponse())
			return false
		} else {
			if nextStateName != "" {
				customNextStepName = nextStateName
				w.EnsureNextState(customNextStepName)
			}
		}
	}

	return true
}

func (w *Engine) Done() *WorkerResponse {
	result := w.Worker.Done(w.Engine)

	return result
}

func (w *Engine) Process(c context.Context, workflowName string, worker Worker) (responses map[string]*WorkerResponse) {
	responses = map[string]*WorkerResponse{}
	if w.LoadWorkflow(c, workflowName, worker) {
		for name, schema := range w.Flows {
			w.Flow = schema
			if w.Start() {
				w.Run()
				response := w.Done()
				responses[name] = &WorkerResponse{
					Error:      response.Error,
					StatusCode: response.StatusCode,
					Response:   response.Response,
				}
			}
		}
	} else {
		response := w.Worker.GetWorkerResponse()
		responses[workflowName] = response
	}

	return responses
}

func (w *Engine) ApplicationContext() context.Context {
	return w.AppContext
}

func (w *Engine) GetFlow() *domain.Flow {
	return w.Flow
}

func (w *Engine) SetPreventLogError(state bool) {
	w.PreventLogError = state
}

func (w *Engine) isPreventedLogError() bool {
	return w.PreventLogError
}

func (w *Engine) GetWorker() *Worker {
	return &w.Worker
}

func (w *Engine) EnsureNextState(name string) {
	for _, transition := range w.Flow.Transitions {
		if transition.To == name {
			var found = false
			for _, from := range transition.From {
				if from == w.Flow.CurrentState.State {
					found = true
				}
			}

			if !found {
				transition.From = append(transition.From, w.Flow.CurrentState.State)
			}
			return
		}
	}
}

func (w *Engine) GetEngineName() string {
	return w.Name
}

func (w *Engine) GetModulesDir() string {
	return w.ModulesPath
}

func (w *Engine) GetApplication() domain.Application {
	return w.Application
}

func (w *Engine) GetWorkflowConfig() domain.WorkflowConfigItem {
	return w.WorkflowConfig
}

// GetWorkflowActions resolves a WSL/SWSL workflow file by name, parses it
// through the standard WSL pipeline (AST + IR graphs), and returns every
// service action referenced by every workflow defined in the module.
//
// The raw argument text is exactly as written in WSL; named-argument names
// are parsed out so callers can match them against transition Go parameter
// names. Terminal states carry `Terminal` = "ok" | "fail".
func (w *Engine) GetWorkflowActions(workflowName string) ([]WorkflowAction, error) {
	mod, graphs, err := w.parseWorkflowSource(workflowName)
	if err != nil {
		return nil, err
	}
	return ExtractActionsFromGraphs(mod, graphs), nil
}

// GetAllWorkflowActions returns the per-workflow action list for every
// workflow currently loaded in the engine (keyed by workflow name). Each
// entry is re-parsed through the standard WSL pipeline so the result
// reflects the current source on disk.
func (w *Engine) GetAllWorkflowActions() map[string][]WorkflowAction {
	out := map[string][]WorkflowAction{}
	for name := range w.Flows {
		actions, err := w.GetWorkflowActions(name)
		if err != nil {
			logger.Warnf("In %s: GetAllWorkflowActions: %s: %v", w.Name, name, err)
			continue
		}
		out[name] = actions
	}
	return out
}

// parseWorkflowSource resolves a workflow file by name and runs it through
// the standard WSL or SWSL parser, returning the AST module and the IR
// graphs map. JSON-only workflows (no source) return an empty module.
func (w *Engine) parseWorkflowSource(workflowName string) (*wsl.Module, map[string]*wsl.Graph, error) {
	f, err := w.GetWorkflowFilePath(workflowName)
	if err != nil {
		return nil, nil, err
	}
	if !f.IsWSL && !f.IsSimplifiedWSL {
		return nil, nil, fmt.Errorf("workflow %q is not a WSL/SWSL source file (path=%s)", workflowName, f.FilePath)
	}

	var data []byte
	if CheckFileExistsInEmbedFS(w.Application.EmbedFS, f.FilePath) {
		data, err = w.Application.EmbedFS.ReadFile(f.FilePath)
	} else {
		data, err = os.ReadFile(f.FilePath)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", f.FilePath, err)
	}

	if f.IsSimplifiedWSL {
		return wsl.ParseAllSimplified(string(data), workflowName)
	}
	return wsl.ParseAll(string(data), workflowName)
}

// ExtractActionsFromGraphs walks the IR graphs (the canonical post-parse
// execution form) and collects every action node into WorkflowAction
// entries. The AST module is only used to iterate workflows in declaration
// order.
func ExtractActionsFromGraphs(mod *wsl.Module, graphs map[string]*wsl.Graph) []WorkflowAction {
	if mod == nil || len(graphs) == 0 {
		return nil
	}
	out := make([]WorkflowAction, 0)
	for _, wf := range mod.Workflows {
		g, ok := graphs[wf.Name]
		if !ok {
			continue
		}
		out = append(out, collectGraphActions(wf.Name, g)...)
	}
	// Include any graphs that weren't listed in mod.Workflows (defensive).
	for name, g := range graphs {
		seen := false
		for _, wf := range mod.Workflows {
			if wf.Name == name {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, collectGraphActions(name, g)...)
		}
	}
	return out
}

func collectGraphActions(workflowName string, g *wsl.Graph) []WorkflowAction {
	if g == nil {
		return nil
	}
	// Walk nodes in a stable order: start first, then alphabetical.
	names := make([]string, 0, len(g.Nodes))
	for name := range g.Nodes {
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		if names[i] == g.Start {
			return true
		}
		if names[j] == g.Start {
			return false
		}
		return names[i] < names[j]
	})

	actions := make([]WorkflowAction, 0, len(names))
	for _, name := range names {
		n := g.Nodes[name]
		if n == nil || n.Action == nil {
			continue
		}
		wa := WorkflowAction{
			Workflow: workflowName,
			State:    name,
			Module:   n.Action.Module,
			Name:     n.Action.Name,
			As:       n.Action.As,
			Params:   append([]string(nil), n.ParamNames...),
		}
		for _, arg := range n.Action.Args {
			raw := strings.TrimSpace(arg.Raw)
			an, av := splitNamedArg(raw)
			wa.Args = append(wa.Args, WorkflowArg{Name: an, Value: av, Raw: raw})
			if an != "" {
				wa.ArgNames = append(wa.ArgNames, an)
			}
		}
		if n.Terminal {
			wa.Terminal = n.TerminalKind
		}
		actions = append(actions, wa)
	}
	return actions
}

// splitNamedArg parses a WSL named-argument expression of the form
// "name: value". Returns empty name when the argument is positional.
func splitNamedArg(raw string) (name, value string) {
	idx := strings.Index(raw, ":")
	if idx < 0 {
		return "", raw
	}
	n := strings.TrimSpace(raw[:idx])
	for _, r := range n {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return "", raw
		}
	}
	if n == "" {
		return "", raw
	}
	return n, strings.TrimSpace(raw[idx+1:])
}

func (w *Engine) wslGraphToSchemaWithExtends(name string, ast *wsl.Module, g *wsl.Graph) (map[string]interface{}, error) {
	schema := wslGraphToSchema(g)
	// Handle WSL-level extends: merge referenced JSON/WSL into the current schema recursively
	return w.wslExtendsLoad(name, ast, schema)
}

func (w *Engine) wslExtendsLoad(name string, ast *wsl.Module, schema map[string]interface{}) (map[string]interface{}, error) {
	if schema == nil {
		schema = make(map[string]interface{})
	}
	if ast != nil && len(ast.Extends) > 0 {
		exname := filepath.Dir(name)
		for _, ex := range ast.Extends {
			extMap, err := w.LoadWorkflowByName(filepath.Join(exname, ex))
			if err != nil {
				return nil, err
			}
			for _, schemaMap := range extMap {
				delete(schemaMap.(map[string]interface{}), "ast")
				schema = helpers.MergeMaps(schema, schemaMap.(map[string]interface{}))
			}
		}
	}

	return schema, nil
}

func (w *Engine) buildSchemaMap(name string, graphs map[string]*wsl.Graph, withPrefix bool, ast *wsl.Module) (schemas map[string]interface{}, err error) {
	var schema map[string]interface{}
	var g *wsl.Graph
	var want string

	schemas = make(map[string]interface{})

	if len(graphs) > 0 {
		for want, g = range graphs {
			currentName := name
			if withPrefix {
				currentName = fmt.Sprintf("@%s", name)
			}
			schema, err = w.wslGraphToSchemaWithExtends(currentName, ast, g)
			if err != nil {
				return nil, err
			}
			schema["ast"] = ast
			schemas[want] = schema
		}
	} else {
		want = ast.Name
		currentName := name
		if withPrefix {
			currentName = fmt.Sprintf("@%s", name)
		}
		schema, err = w.wslExtendsLoad(currentName, ast, schema)
		if err != nil {
			return nil, err
		}
		schema["ast"] = ast
		schemas[want] = schema
	}

	return schemas, nil
}

func (w *Engine) updateSchemas(schemas map[string]interface{}, resolvers []string, constants map[string]interface{}) map[string]interface{} {
	var want string
	var graph interface{}
	var schema map[string]interface{}

	for want, graph = range schemas {
		schema = graph.(map[string]interface{})
		if schema["resolvers"] == nil {
			schema["resolvers"] = []string{}
		}
		if schema["constants"] == nil {
			schema["constants"] = map[string]interface{}{}
		}
		if resolvers == nil {
			resolvers = []string{}
		}
		if constants == nil {
			constants = map[string]interface{}{}
		}
		schema["resolvers"] = helpers.AppendUnique(schema["resolvers"].([]string), resolvers)
		schema["constants"] = helpers.MergeMaps(schema["constants"].(map[string]interface{}), constants)
		schemas[want] = schema
	}

	return schemas
}

// CheckFileExistsInEmbedFS checks if a file path exists within the embedded file system.
func CheckFileExistsInEmbedFS(content embed.FS, path string) bool {
	// The embedded content (embed.FS) implements fs.FS.
	// We use the fs.FS.Open() method to check for existence.
	_, err := content.Open(path)

	if err != nil {
		// os.IsNotExist checks if the error wraps or is exactly the "file not found" error.
		if os.IsNotExist(err) {
			// The error indicates the file does not exist.
			return false
		}

		// Handle other potential errors (e.g., permission denied, I/O error)
		// For this basic check, we treat any non-NotExist error as "not found"
		// or an error we cannot proceed with, depending on requirements.
		fmt.Printf("[DEBUG] Could not check '%s' due to a different error: %v\n", path, err)
		return false
	}

	// If Open() returned no error, the file exists.
	return true
}
