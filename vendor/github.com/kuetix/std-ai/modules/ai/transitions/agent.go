package transitions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	engWorkflow "github.com/kuetix/engine/engine/workflow"
)

const (
	agentActionFinal = "final"
	agentActionTool  = "tool"

	agentToolWorkflow = "workflow"
	agentToolFeature  = "feature"
	agentToolSolution = "solution"
	agentToolHTTP     = "http"
	agentToolMCP      = "mcp"
	agentToolExternal = "external"
)

type agentTransitions struct {
	engWorkflow.BaseServiceTransition
}

func NewAgentTransitions() interfaces.ServiceTransitions {
	return &agentTransitions{}
}

type agentToolSpec struct {
	Name        string
	Kind        string
	Target      string
	Adapter     string
	Description string
	Input       map[string]interface{}
	Options     map[string]interface{}
}

type agentToolCall struct {
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

type agentDecision struct {
	Action string         `json:"action"`
	Final  string         `json:"final"`
	Tool   *agentToolCall `json:"tool"`
}

type agentTurn struct {
	Iteration int                    `json:"iteration"`
	Kind      string                 `json:"kind"`
	Data      map[string]interface{} `json:"data"`
}

func normalizeToolKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", agentToolWorkflow:
		return agentToolWorkflow
	case agentToolFeature:
		return agentToolFeature
	case agentToolSolution:
		return agentToolSolution
	case agentToolHTTP:
		return agentToolHTTP
	case agentToolMCP:
		return agentToolMCP
	case agentToolExternal:
		return agentToolExternal
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

func copyMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func dropKeys(input map[string]interface{}, keys ...string) map[string]interface{} {
	out := copyMap(input)
	for _, key := range keys {
		delete(out, key)
	}
	return out
}

func parseTools(tools map[string]interface{}) (map[string]agentToolSpec, error) {
	result := map[string]agentToolSpec{}
	for toolName, raw := range tools {
		specMap, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("tool %q must be an object", toolName)
		}

		kind := normalizeToolKind(firstNonEmpty(valueAsString(specMap["kind"]), valueAsString(specMap["type"])))
		switch kind {
		case agentToolWorkflow, agentToolFeature, agentToolSolution, agentToolHTTP, agentToolMCP, agentToolExternal:
		default:
			return nil, fmt.Errorf("tool %q has unsupported kind %q", toolName, kind)
		}

		target := ""
		switch kind {
		case agentToolFeature:
			target = firstNonEmpty(valueAsString(specMap["target"]), valueAsString(specMap["feature"]), valueAsString(specMap["name"]))
		case agentToolSolution:
			target = firstNonEmpty(valueAsString(specMap["target"]), valueAsString(specMap["solution"]), valueAsString(specMap["name"]))
		case agentToolHTTP:
			target = firstNonEmpty(valueAsString(specMap["target"]), valueAsString(specMap["url"]))
		case agentToolMCP:
			target = firstNonEmpty(valueAsString(specMap["target"]), valueAsString(specMap["endpoint"]), valueAsString(specMap["server"]), valueAsString(specMap["command"]))
		default:
			target = firstNonEmpty(valueAsString(specMap["target"]), valueAsString(specMap["workflow"]), valueAsString(specMap["name"]))
		}
		if target == "" {
			return nil, fmt.Errorf("tool %q must define a target", toolName)
		}

		adapter := ""
		if kind == agentToolExternal {
			adapter = strings.TrimSpace(valueAsString(specMap["adapter"]))
			if adapter == "" {
				return nil, fmt.Errorf("tool %q with kind %q must define adapter", toolName, kind)
			}
		}

		input := map[string]interface{}{}
		if rawInput, ok := specMap["input"]; ok && rawInput != nil {
			parsedInput, ok := rawInput.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("tool %q input must be an object", toolName)
			}
			input = copyMap(parsedInput)
		}

		options := dropKeys(specMap,
			"kind", "type", "target", "workflow", "feature", "solution", "url", "endpoint", "server", "command",
			"name", "description", "input", "adapter",
		)

		if kind == agentToolMCP {
			remoteToolName := firstNonEmpty(valueAsString(specMap["tool"]), valueAsString(specMap["remoteTool"]))
			if remoteToolName == "" {
				return nil, fmt.Errorf("tool %q with kind %q must define tool", toolName, kind)
			}
			options["tool"] = remoteToolName
		}

		result[toolName] = agentToolSpec{
			Name:        toolName,
			Kind:        kind,
			Target:      target,
			Adapter:     adapter,
			Description: strings.TrimSpace(valueAsString(specMap["description"])),
			Input:       input,
			Options:     options,
		}
	}
	return result, nil
}

func valueAsString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		if value == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func sortedToolNames(toolMap map[string]agentToolSpec) []string {
	names := make([]string, 0, len(toolMap))
	for name := range toolMap {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func formatToolsForPrompt(toolMap map[string]agentToolSpec) string {
	if len(toolMap) == 0 {
		return "No tools are available. You must return {\"action\":\"final\",\"final\":\"...\"}."
	}

	descriptions := make([]map[string]interface{}, 0, len(toolMap))
	for _, name := range sortedToolNames(toolMap) {
		tool := toolMap[name]
		descriptions = append(descriptions, map[string]interface{}{
			"name":         tool.Name,
			"kind":         tool.Kind,
			"target":       tool.Target,
			"adapter":      tool.Adapter,
			"description":  tool.Description,
			"defaultInput": tool.Input,
			"options":      tool.Options,
		})
	}

	encoded, err := json.MarshalIndent(descriptions, "", "  ")
	if err != nil {
		return "Failed to describe tools."
	}
	return string(encoded)
}

func formatTurnsForPrompt(turns []interface{}) string {
	if len(turns) == 0 {
		return "[]"
	}
	encoded, err := json.MarshalIndent(turns, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func turnsToInterfaces(turns []agentTurn) []interface{} {
	result := make([]interface{}, 0, len(turns))
	for _, turn := range turns {
		result = append(result, map[string]interface{}{
			"iteration": turn.Iteration,
			"kind":      turn.Kind,
			"data":      turn.Data,
		})
	}
	return result
}

func buildAgentPrompt(goal string, turns []interface{}, toolMap map[string]agentToolSpec) string {
	return strings.TrimSpace(fmt.Sprintf(`
You are a workflow agent operating inside Kuetix.

Goal:
%s

Available allowlisted tools:
%s

Prior turns:
%s

Return ONLY valid JSON with one of these shapes:
{"action":"final","final":"your final answer"}
{"action":"tool","tool":{"name":"tool_name","input":{"key":"value"}}}

Rules:
- You may only call tools from the allowlisted tool registry above.
- Use "tool" only when a listed tool is required.
- Use only one tool per response.
- Keep JSON compact and valid.
- Do not wrap JSON in markdown fences.
`, goal, formatToolsForPrompt(toolMap), formatTurnsForPrompt(turns)))
}

func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 3 {
			text = strings.Join(lines[1:len(lines)-1], "\n")
			text = strings.TrimSpace(strings.TrimPrefix(text, "json"))
		}
	}

	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

func parseAgentDecision(text string) (agentDecision, error) {
	var decision agentDecision
	jsonText := extractJSONObject(text)
	if jsonText == "" {
		return decision, fmt.Errorf("agent response did not contain JSON")
	}
	if err := json.Unmarshal([]byte(jsonText), &decision); err != nil {
		return decision, fmt.Errorf("failed to parse agent JSON: %w", err)
	}
	decision.Action = strings.TrimSpace(decision.Action)
	switch decision.Action {
	case agentActionFinal:
		if strings.TrimSpace(decision.Final) == "" {
			return decision, fmt.Errorf("final action requires final text")
		}
	case agentActionTool:
		if decision.Tool == nil {
			return decision, fmt.Errorf("tool action requires tool payload")
		}
		decision.Tool.Name = strings.TrimSpace(decision.Tool.Name)
		if decision.Tool.Name == "" {
			return decision, fmt.Errorf("tool action requires tool name")
		}
		if decision.Tool.Input == nil {
			decision.Tool.Input = map[string]interface{}{}
		}
	default:
		return decision, fmt.Errorf("unsupported agent action %q", decision.Action)
	}
	return decision, nil
}

func mergeMaps(base map[string]interface{}, override map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for key, value := range base {
		result[key] = value
	}
	for key, value := range override {
		result[key] = value
	}
	return result
}

func bestResponse(responses map[string]*engWorkflow.WorkerResponse, preferred string) *engWorkflow.WorkerResponse {
	if responses == nil {
		return nil
	}
	if preferred != "" {
		if response, ok := responses[preferred]; ok {
			return response
		}
	}
	if base := preferred[strings.LastIndex(preferred, "/")+1:]; base != "" {
		if response, ok := responses[base]; ok {
			return response
		}
	}
	for _, response := range responses {
		return response
	}
	return nil
}

func withTemporaryArgs(p *engWorkflow.WorkerSessionContext, args map[string]interface{}, fn func() error) error {
	context := p.WorkflowContext.Context()
	previousArgs, hadArgs := (*context)["args"]

	overwritten := map[string]interface{}{}
	hadKey := map[string]bool{}
	for key, value := range args {
		if current, ok := (*context)[key]; ok {
			hadKey[key] = true
			overwritten[key] = current
		}
		(*context)[key] = value
	}
	(*context)["args"] = args

	defer func() {
		if hadArgs {
			(*context)["args"] = previousArgs
		} else {
			delete(*context, "args")
		}
		for key := range args {
			if hadKey[key] {
				(*context)[key] = overwritten[key]
			} else {
				delete(*context, key)
			}
		}
	}()

	return fn()
}

func executeWorkflowTool(p *engWorkflow.WorkerSessionContext, tool agentToolSpec, input map[string]interface{}) (map[string]interface{}, error) {
	args := mergeMaps(tool.Input, input)

	var (
		responses map[string]*engWorkflow.WorkerResponse
		err       error
	)

	run := func() error {
		switch tool.Kind {
		case agentToolFeature:
			responses, err = engWorkflow.ExecuteWithRunnerAndSharedContext(p, "feature", tool.Target)
		case agentToolSolution:
			responses, err = engWorkflow.ExecuteWithRunnerAndSharedContext(p, "solution", tool.Target)
		default:
			responses, err = engWorkflow.ExecuteWithRunnerAndSharedContext(p, "workflow", tool.Target)
		}
		return err
	}

	if err := withTemporaryArgs(p, args, run); err != nil {
		return nil, fmt.Errorf("tool %q failed to start: %w", tool.Name, err)
	}

	response := bestResponse(responses, tool.Target)
	if response == nil {
		return nil, fmt.Errorf("tool %q returned no response", tool.Name)
	}

	result := map[string]interface{}{
		"name":       tool.Name,
		"kind":       tool.Kind,
		"target":     tool.Target,
		"input":      args,
		"statusCode": response.StatusCode,
		"success":    response.IsSuccess(),
		"response":   response.Response,
	}
	if response.Error != nil {
		result["error"] = response.Error.Error()
		return result, fmt.Errorf("tool %q failed: %s", tool.Name, response.Error.Error())
	}
	return result, nil
}

func defaultAgentSystemPrompt(systemPrompt string) string {
	base := strings.TrimSpace(systemPrompt)
	if base == "" {
		base = "You are a helpful AI workflow agent."
	}
	return base + " Always follow the required JSON response contract exactly."
}

// BuildPrompt parses the tool registry and assembles the model prompt for one
// agent turn. It returns the user prompt text, the effective system prompt, and
// the allowlisted tool names, without calling the model.
func (t *agentTransitions) BuildPrompt(goal, systemPrompt string, tools map[string]interface{}, turns []interface{}) (r domain.FlowStepResult) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		r.Error = fmt.Errorf("goal is required")
		r.StatusCode = http.StatusBadRequest
		return
	}

	toolMap, err := parseTools(tools)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"prompt":       buildAgentPrompt(goal, turns, toolMap),
		"systemPrompt": defaultAgentSystemPrompt(systemPrompt),
		"allowedTools": sortedToolNames(toolMap),
	}
	return
}

// ParseDecision parses a raw model response into a validated agent decision and
// enforces the tool allowlist. On failure it returns the raw text and the
// allowed tool names so the caller can retry or report.
func (t *agentTransitions) ParseDecision(raw string, tools map[string]interface{}) (r domain.FlowStepResult) {
	toolMap, err := parseTools(tools)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	decision, err := parseAgentDecision(raw)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadGateway
		r.Response = map[string]interface{}{
			"raw":          raw,
			"allowedTools": sortedToolNames(toolMap),
		}
		return
	}

	if decision.Tool != nil {
		if _, ok := toolMap[decision.Tool.Name]; !ok {
			r.Error = fmt.Errorf("agent requested non-allowlisted tool %q", decision.Tool.Name)
			r.StatusCode = http.StatusForbidden
			r.Response = map[string]interface{}{
				"raw":          raw,
				"allowedTools": sortedToolNames(toolMap),
			}
			return
		}
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"raw":          raw,
		"action":       decision.Action,
		"final":        decision.Final,
		"tool":         decision.Tool,
		"allowedTools": sortedToolNames(toolMap),
	}
	return
}

// Decide asks the model for the next allowlisted agent action and returns a parsed decision payload.
// It composes BuildPrompt -> prompt.Prompt -> ParseDecision.
func (t *agentTransitions) Decide(goal, provider, model, apiKey, baseURL, systemPrompt string, temperature float64, maxTokens int, tools map[string]interface{}, turns []interface{}, headers map[string]interface{}) (r domain.FlowStepResult) {
	buildResult := t.BuildPrompt(goal, systemPrompt, tools, turns)
	if !buildResult.Success {
		return buildResult
	}
	buildMap, ok := buildResult.Response.(map[string]interface{})
	if !ok {
		r.Error = fmt.Errorf("build prompt returned invalid payload")
		r.StatusCode = http.StatusInternalServerError
		return
	}
	promptText, _ := buildMap["prompt"].(string)
	effectiveSystemPrompt, _ := buildMap["systemPrompt"].(string)

	promptClient := &promptTransitions{}
	promptResult := promptClient.Prompt(provider, promptText, model, apiKey, baseURL, effectiveSystemPrompt, temperature, maxTokens, headers)
	if !promptResult.Success {
		return promptResult
	}

	responseMap, ok := promptResult.Response.(map[string]interface{})
	if !ok {
		r.Error = fmt.Errorf("agent prompt returned invalid response payload")
		r.StatusCode = http.StatusBadGateway
		return
	}
	rawText, _ := responseMap["text"].(string)

	parseResult := t.ParseDecision(rawText, tools)
	if !parseResult.Success {
		return parseResult
	}
	parseMap, ok := parseResult.Response.(map[string]interface{})
	if !ok {
		r.Error = fmt.Errorf("parse decision returned invalid payload")
		r.StatusCode = http.StatusBadGateway
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"provider":     responseMap["provider"],
		"model":        responseMap["model"],
		"raw":          rawText,
		"action":       parseMap["action"],
		"final":        parseMap["final"],
		"tool":         parseMap["tool"],
		"allowedTools": parseMap["allowedTools"],
	}
	return
}

// resolveToolSpec looks up a tool by name in the parsed allowlist. On failure it
// returns ok=false with a populated error result.
func resolveToolSpec(tool string, tools map[string]interface{}) (agentToolSpec, domain.FlowStepResult, bool) {
	toolMap, err := parseTools(tools)
	if err != nil {
		return agentToolSpec{}, domain.FlowStepResult{Error: err, StatusCode: http.StatusBadRequest}, false
	}
	spec, ok := toolMap[strings.TrimSpace(tool)]
	if !ok {
		return agentToolSpec{}, domain.FlowStepResult{
			Error:      fmt.Errorf("tool %q is not allowlisted", tool),
			StatusCode: http.StatusForbidden,
			Response:   map[string]interface{}{"allowedTools": sortedToolNames(toolMap)},
		}, false
	}
	return spec, domain.FlowStepResult{}, true
}

// runExternalTool executes an http/mcp/external tool spec via the external
// adapter and shapes the standard tool-result envelope.
func runExternalTool(spec agentToolSpec, input map[string]interface{}) (map[string]interface{}, error) {
	external := &externalTransitions{}
	adapter := spec.Kind
	if spec.Kind == agentToolExternal {
		adapter = spec.Adapter
	}
	execution := external.Execute(adapter, spec.Target, spec.Options, mergeMaps(spec.Input, input))

	result := map[string]interface{}{
		"name":       spec.Name,
		"kind":       spec.Kind,
		"adapter":    adapter,
		"target":     spec.Target,
		"statusCode": execution.StatusCode,
		"success":    execution.Success,
		"response":   execution.Response,
	}
	if responseMap, ok := execution.Response.(map[string]interface{}); ok {
		result["response"] = responseMap
	}
	if execution.Error != nil {
		result["error"] = execution.Error.Error()
		return result, execution.Error
	}
	return result, nil
}

// ExecuteWorkflowTool dispatches a single allowlisted workflow/feature/solution
// tool call from the agent registry.
func (t *agentTransitions) ExecuteWorkflowTool(p *engWorkflow.WorkerSessionContext, tool string, tools map[string]interface{}, input map[string]interface{}) (r domain.FlowStepResult) {
	if p == nil {
		r.Error = fmt.Errorf("worker session context is required")
		r.StatusCode = http.StatusBadRequest
		return
	}
	spec, missing, ok := resolveToolSpec(tool, tools)
	if !ok {
		return missing
	}
	switch spec.Kind {
	case agentToolWorkflow, agentToolFeature, agentToolSolution:
	default:
		r.Error = fmt.Errorf("tool %q has kind %q which is not a workflow tool", tool, spec.Kind)
		r.StatusCode = http.StatusBadRequest
		return
	}

	result, err := executeWorkflowTool(p, spec, input)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadGateway
		r.Response = result
		return
	}
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = result
	return
}

// ExecuteExternalTool dispatches a single allowlisted http/mcp/external tool call
// from the agent registry.
func (t *agentTransitions) ExecuteExternalTool(tool string, tools map[string]interface{}, input map[string]interface{}) (r domain.FlowStepResult) {
	spec, missing, ok := resolveToolSpec(tool, tools)
	if !ok {
		return missing
	}
	switch spec.Kind {
	case agentToolHTTP, agentToolMCP, agentToolExternal:
	default:
		r.Error = fmt.Errorf("tool %q has kind %q which is not an external tool", tool, spec.Kind)
		r.StatusCode = http.StatusBadRequest
		return
	}

	result, err := runExternalTool(spec, input)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadGateway
		r.Response = result
		return
	}
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = result
	return
}

// ExecuteTool dispatches a single allowlisted tool call, routing to the
// workflow or external executor based on the tool kind.
func (t *agentTransitions) ExecuteTool(p *engWorkflow.WorkerSessionContext, tool string, tools map[string]interface{}, input map[string]interface{}) (r domain.FlowStepResult) {
	spec, missing, ok := resolveToolSpec(tool, tools)
	if !ok {
		return missing
	}
	switch spec.Kind {
	case agentToolWorkflow, agentToolFeature, agentToolSolution:
		return t.ExecuteWorkflowTool(p, tool, tools, input)
	default:
		return t.ExecuteExternalTool(tool, tools, input)
	}
}

// AppendTurn appends a single turn to the running transcript and returns the
// extended transcript together with its length. It is the accumulator used by
// the decomposed agent loop workflow to thread turns across iterations.
func (t *agentTransitions) AppendTurn(turns []interface{}, iteration int, kind string, data interface{}) (r domain.FlowStepResult) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		r.Error = fmt.Errorf("kind is required")
		r.StatusCode = http.StatusBadRequest
		return
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		dataMap = map[string]interface{}{}
		if data != nil {
			dataMap["value"] = data
		}
	}

	next := make([]interface{}, 0, len(turns)+1)
	next = append(next, turns...)
	next = append(next, map[string]interface{}{
		"iteration": iteration,
		"kind":      kind,
		"data":      dataMap,
	})

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"turns": next,
		"count": len(next),
	}
	return
}

// Run executes an allowlisted custom AI agent loop and delegates decisions/tool execution to Decide/ExecuteTool.
func (t *agentTransitions) Run(p *engWorkflow.WorkerSessionContext, goal, provider, model, apiKey, baseURL, systemPrompt string, maxIterations int, temperature float64, maxTokens int, tools map[string]interface{}, headers map[string]interface{}) (r domain.FlowStepResult) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		r.Error = fmt.Errorf("goal is required")
		r.StatusCode = http.StatusBadRequest
		return
	}
	if maxIterations <= 0 {
		maxIterations = 8
	}
	if maxIterations > 32 {
		r.Error = fmt.Errorf("maxIterations must be less than or equal to 32")
		r.StatusCode = http.StatusBadRequest
		return
	}

	parsedTools, err := parseTools(tools)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	turns := make([]agentTurn, 0, maxIterations*2)
	for iteration := 1; iteration <= maxIterations; iteration++ {
		decisionResult := t.Decide(goal, provider, model, apiKey, baseURL, systemPrompt, temperature, maxTokens, tools, turnsToInterfaces(turns), headers)
		if !decisionResult.Success {
			r = decisionResult
			if r.Response == nil {
				r.Response = map[string]interface{}{}
			}
			if responseMap, ok := r.Response.(map[string]interface{}); ok {
				responseMap["iterations"] = iteration
				responseMap["turns"] = turnsToInterfaces(turns)
			}
			return
		}

		decisionMap, ok := decisionResult.Response.(map[string]interface{})
		if !ok {
			r.Error = fmt.Errorf("decide returned invalid payload")
			r.StatusCode = http.StatusBadGateway
			return
		}

		turns = append(turns, agentTurn{
			Iteration: iteration,
			Kind:      "assistant",
			Data: map[string]interface{}{
				"raw":    decisionMap["raw"],
				"action": decisionMap["action"],
				"tool":   decisionMap["tool"],
				"final":  decisionMap["final"],
			},
		})

		action, _ := decisionMap["action"].(string)
		if action == agentActionFinal {
			r.Success = true
			r.StatusCode = http.StatusOK
			r.Response = map[string]interface{}{
				"provider":   decisionMap["provider"],
				"model":      decisionMap["model"],
				"iterations": iteration,
				"final":      decisionMap["final"],
				"turns":      turnsToInterfaces(turns),
				"tools":      sortedToolNames(parsedTools),
			}
			return
		}

		toolPayload, ok := decisionMap["tool"].(map[string]interface{})
		if !ok {
			r.Error = fmt.Errorf("decide did not return a valid tool payload")
			r.StatusCode = http.StatusBadGateway
			r.Response = map[string]interface{}{
				"iterations": iteration,
				"turns":      turnsToInterfaces(turns),
			}
			return
		}

		toolName, _ := toolPayload["name"].(string)
		toolInput, _ := toolPayload["input"].(map[string]interface{})
		executionResult := t.ExecuteTool(p, toolName, tools, toolInput)
		turns = append(turns, agentTurn{
			Iteration: iteration,
			Kind:      "tool",
			Data: map[string]interface{}{
				"name":   toolName,
				"result": executionResult.Response,
			},
		})
		if !executionResult.Success {
			r = executionResult
			if r.Response == nil {
				r.Response = map[string]interface{}{}
			}
			if responseMap, ok := r.Response.(map[string]interface{}); ok {
				responseMap["iterations"] = iteration
				responseMap["turns"] = turnsToInterfaces(turns)
			}
			return
		}
	}

	r.Error = fmt.Errorf("agent reached maxIterations without returning a final answer")
	r.StatusCode = http.StatusGatewayTimeout
	r.Response = map[string]interface{}{
		"iterations": maxIterations,
		"turns":      turnsToInterfaces(turns),
	}
	return
}
