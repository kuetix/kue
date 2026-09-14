package workflow

import (
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/domain/issues"
	localHelpers "github.com/kuetix/engine/engine/helpers"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

type Worker interface {
	Context() *map[string]interface{}
	SetContext(context *map[string]interface{})

	Start(w EngineInterface) bool
	InitResolvers(resolvers []string)
	PrepareContext(w EngineInterface, flow *domain.Flow) bool
	ProcessState(w EngineInterface, flow *domain.Flow) (bool, string)
	Done(w EngineInterface) *WorkerResponse
	ProcessStateError(w EngineInterface, flow *domain.Flow, response *WorkerResponse)
	SetResponse(response any, statusCode ...int)
	GetResponse() (response any)
	SetLastResponse(response any, statusCode int)
	SetLastStatusCode(statusCode int)
	GetLastResponse() (response any)
	GetWorkerResponse() (response *WorkerResponse)
	GetWorkerLastResponse() (response *WorkerResponse)
	HandleError(err interface{}, statusCode int) (success bool)
	SetError(error *issues.Issue, statusCode ...int) bool
	SetErrors(errors *issues.Issues, statusCode ...int) bool
	MergeIssues(errors *issues.Issues, statusCode ...int) bool
	GetError() (response *issues.Issues)
	SetStatusCode(statusCode int)
	GetStatusCode() int
	SetDebug(debug bool)
	IsDebug() bool
	GetWorkflowContext() WorkerContext
	CleanErrors() bool
}

type workflowWorker struct {
	WorkflowContext       WorkerContext
	Transitions           map[string]interfaces.ServiceTransitions
	TransitionsResolved   map[string]*ServiceTransitionMapping
	TransitionsNamespaces []string
	Response              *WorkerResponse
	LastResponse          *WorkerResponse
	Debug                 bool
	Options               map[string]interface{}
}

func NewWorkflowWorker(workflowContext WorkerContext, transitions map[string]interfaces.ServiceTransitions, options ...map[string]interface{}) Worker {
	opts := helpers.MergeMapsLevel0(options...)
	debugOpt, ok := opts["debug"]
	if !ok {
		debugOpt = false
	}
	worker := &workflowWorker{
		WorkflowContext:     workflowContext,
		Transitions:         transitions,
		TransitionsResolved: map[string]*ServiceTransitionMapping{},
		Response: &WorkerResponse{
			Error:    nil,
			Response: nil,
		},
		LastResponse: &WorkerResponse{
			Error:    nil,
			Response: nil,
		},
		Debug:   debugOpt.(bool),
		Options: opts,
	}

	return worker
}

func (baseWorker *workflowWorker) Context() *map[string]interface{} {
	return baseWorker.WorkflowContext.Context()
}

func (baseWorker *workflowWorker) SetContext(context *map[string]interface{}) {
	baseWorker.WorkflowContext.SetContext(context)
}

func (baseWorker *workflowWorker) Start(w EngineInterface) bool {
	baseWorker.InitResolvers(w.GetResolvers())

	return true
}

func (baseWorker *workflowWorker) InitResolvers(resolvers []string) {
	if resolvers != nil && len(resolvers) > 0 {
		for _, resolver := range resolvers {
			if baseWorker.Debug {
				logger.Debug("Resolver: ", defines.TransitionPrefix+resolver)
			}

			if baseWorker.TransitionsNamespaces != nil || len(baseWorker.TransitionsNamespaces) > 0 {
				baseWorker.TransitionsNamespaces = []string{}
			}
			ns := filepath.Dir(resolver)
			baseWorker.TransitionsNamespaces = append(baseWorker.TransitionsNamespaces, ns)
			func() {
				defer func() {
					if r := recover(); r != nil {
						var keys []string
						for i := range di.FactoryContainer {
							keys = append(keys, i)
						}
						logger.Errorf("Factory container keys: %v", keys)
						logger.Panicf("Recovered in InitResolvers: %v", r)
					}
				}()
				t := di.Resolve(defines.TransitionPrefix + resolver).(ServiceTransitionMapping)
				baseWorker.TransitionsResolved[resolver] = &t
				baseWorker.Transitions[resolver] = t.Impl
			}()
		}
	}

}

func (baseWorker *workflowWorker) PrepareContext(w EngineInterface, flow *domain.Flow) bool {
	baseWorker.WorkflowContext.SetValue("workflow.Flow", flow)
	baseWorker.WorkflowContext.SetValue("workflow.EngineInterface", w)
	baseWorker.WorkflowContext.SetValue("workflow.Worker", baseWorker)

	return true
}

func (baseWorker *workflowWorker) ProcessState(w EngineInterface, flow *domain.Flow) (bool, string) {
	comments := strings.Split(flow.CurrentState.State, "#")
	command := strings.SplitN(comments[0], ":", 2)
	context := baseWorker.WorkflowContext.Context()
	if parentFlow, ok := (*context)["Parent"]; ok {
		flow.Parent = parentFlow.(*domain.Flow)
		delete(*context, "Parent")
	}
	workerSessionContext := WorkerSessionContext{
		WorkflowContext: baseWorker.WorkflowContext,
		Worker:          baseWorker,
		Flow:            flow,
		Engine:          w,
	}

	// Parallel fork/join states are handled by the engine itself, not by a
	// single transition call.
	if flow.CurrentTransition != nil {
		if flow.CurrentTransition.ParallelCount > 0 {
			return baseWorker.processParallelFork(&workerSessionContext)
		}
		if flow.CurrentTransition.WaitJoin != "" {
			return baseWorker.processParallelWait(&workerSessionContext)
		}
	}

	// Bind call arguments to target state parameters (if provided)
	if flow.CurrentTransition != nil && flow.CurrentState != nil && flow.CurrentTransition.Options != nil {
		// prefer param names from transition; fallback to state _params
		var paramNames []string
		if pn, ok := flow.CurrentTransition.Options["_call.paramNames"]; ok {
			if arr, ok := pn.([]interface{}); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok {
						paramNames = append(paramNames, s)
					}
				}
			}
		}
		if len(paramNames) == 0 && flow.CurrentState.Options != nil {
			if pn, ok := flow.CurrentState.Options["_params"]; ok {
				if arr, ok := pn.([]interface{}); ok {
					for _, v := range arr {
						if s, ok := v.(string); ok {
							paramNames = append(paramNames, s)
						}
					}
				}
			}
		}
		if len(paramNames) > 0 {
			var argsList []string
			if amap, ok := flow.CurrentTransition.Options["_call.args.map"]; ok {
				if m, ok := amap.(map[string]interface{}); ok {
					from := "_"
					if flow.LastTrace != nil && flow.LastTrace.GetFrom() != "" {
						from = flow.LastTrace.GetFrom()
					}
					// Try exact key first, then try without hash suffix (e.g., "state#1" -> "state"), then fallback to "_"
					candidates := []string{from}
					if idx := strings.Index(from, "#"); idx > 0 {
						candidates = append(candidates, from[:idx])
					}
					if from != "_" {
						candidates = append(candidates, "_")
					}
					for _, key := range candidates {
						if raw, ok := m[key]; ok {
							if arr, ok := raw.([]interface{}); ok {
								for _, v := range arr {
									if s, ok := v.(string); ok {
										argsList = append(argsList, s)
									}
								}
							}
							break
						}
					}
				}
			}
			// If no args found for predecessor, leave empty (all params optional)
			if len(argsList) > 0 {
				// Evaluate and bind positionally
				for i, pname := range paramNames {
					if i >= len(argsList) {
						break
					}
					raw := strings.TrimSpace(argsList[i])
					// Evaluate via context: treat as property token if not quoted/number/bool
					val := interface{}(raw)
					// try parse quoted string
					if len(raw) >= 2 && ((raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'')) {
						val = strings.Trim(raw, "\"'")
					} else {
						// try number
						if iv, err := strconv.ParseInt(raw, 10, 64); err == nil {
							val = int(iv)
						} else if fv, err := strconv.ParseFloat(raw, 64); err == nil {
							val = fv
						} else if strings.EqualFold(raw, "true") || strings.EqualFold(raw, "false") {
							val = strings.EqualFold(raw, "true")
						} else {
							// resolve from context using ParseProperty on <<raw>>
							_, v, err := workerSessionContext.ParseProperty("<<" + raw + ">>")
							if err == nil && v != nil {
								val = v
							}
						}
					}
					baseWorker.WorkflowContext.SetValue(pname, val)
				}
			}
		}
	}

	if flow.CurrentTransition.If != nil {
		conditionProp := *flow.CurrentTransition.If
		condition, err := workerSessionContext.Parser.ParseTemplate(conditionProp)
		if !workerSessionContext.Worker.HandleError(err, http.StatusInternalServerError) {
			return false, ""
		}
		if condition == "false" {
			if flow.CurrentTransition.Else != nil {
				elseConditionProp := *flow.CurrentTransition.Else
				elseCondition, err := workerSessionContext.Parser.ParseTemplate(elseConditionProp)
				if !workerSessionContext.Worker.HandleError(err, http.StatusInternalServerError) {
					return false, ""
				}
				nextStep := elseCondition
				return true, nextStep
			}
			return true, ""
		}
	}

	if len(command) > 0 {
		if command[0] == "workflow" || command[0] == "feature" || command[0] == "solution" {
			flowType := command[0] // Get the flow type (workflow, feature, or solution)
			workerSessionContext.StartStep()

			// Check if this is a simple name or a full path with arguments
			// Format can be: "workflow simple_name" or "workflow namespace/path.workflow_name"
			var name string
			if len(command) > 1 && command[1] != "" {
				// Parse the name - it could be a property reference or a path
				prop := fmt.Sprintf("%s|parse", command[1])
				foundKey, foundName, err := workerSessionContext.GetProperty(prop)
				if err != nil || foundName == foundKey {
					// Not a property, use as-is (could be a path like namespace/path.workflow_name)
					name = command[1]
				} else {
					// It was a property, use the resolved value
					name = foundName.(string)
				}
			} else {
				// No name provided, use the type as name
				name = command[0]
			}

			if baseWorker.Debug {
				logger.Debug(fmt.Sprintf("%s Found, config: %s", flowType, name))
			}

			(*context)["Parent"] = flow

			// Use the appropriate runner based on the flow type
			responses, runErr := ExecuteWithRunnerAndSharedContext(&workerSessionContext, flowType, name)
			if runErr != nil {
				baseWorker.HandleError(runErr, http.StatusInternalServerError)
				return false, ""
			}

			baseWorker.SetResponse(responses)
			if responses != nil {
				for _, resp := range responses {
					if resp.IsError() {
						baseWorker.HandleError(resp.Error, resp.StatusCode)
						return false, ""
					}
				}
			}

			return true, ""
		}
	}
	callPath := comments[0]
	path := strings.Split(callPath, "/")
	methodName := path[len(path)-1]
	transitionsName := strings.Join(path[:len(path)-1], "/")
	var workerTransitions *ServiceTransitionMapping
	if transitionsName != "" {
		if _, ok := baseWorker.TransitionsResolved[transitionsName]; !ok {
			for _, ns := range baseWorker.TransitionsNamespaces {
				tnNs := filepath.ToSlash(filepath.Join(ns, transitionsName))
				if _, ok := baseWorker.TransitionsResolved[tnNs]; ok {
					workerTransitions = baseWorker.TransitionsResolved[tnNs]
					break
				}
			}
		} else {
			workerTransitions = baseWorker.TransitionsResolved[transitionsName]
		}
	} else {
		for tn, wt := range baseWorker.TransitionsResolved {
			transitionsName = tn
			workerTransitions = wt
			break
		}
	}
	if baseWorker.Debug {
		logger.Debug("TransitionsName", transitionsName)
		logger.Debug("MethodName", methodName)
	}
	if workerTransitions == nil {
		panic(fmt.Sprintf("Please add %s to \"resolvers\" in workflow because transitions %s not found to call method %s in resolvers: %v with trace: %s: %v", transitionsName, transitionsName, methodName, flow.Resolvers, flow.ConfigName, flow.GetTraceString()))
	}
	valueOf := reflect.ValueOf(workerTransitions.Impl)
	if baseWorker.Debug {
		logger.Debug("WorkerTransitions: ", valueOf.Type(), valueOf.Kind(), valueOf)
	}
	if valueOf.Type() == nil {
		panic(fmt.Sprintf("Please add %s to \"resolvers\" in workflow because transitions %s not found to call method %s in resolvers: %v with trace: %s: %v", transitionsName, transitionsName, methodName, flow.Resolvers, flow.ConfigName, flow.GetTraceString()))
	}
	method := valueOf.MethodByName(methodName)
	var nextStateNameOrError string
	if method.IsValid() && method.Kind() == reflect.Func || flow.CurrentTransition.SkipTo != nil {
		if baseWorker.Debug {
			logger.Debug("Method Found")
		}
		// Call the method and print the result
		args := []reflect.Value{reflect.ValueOf(&workerSessionContext)}
		var err error
		var results []reflect.Value
		var isDone bool
		if flow.CurrentTransition.SkipTo == nil {
			logger.Debug(fmt.Sprintf("-> %s/%s %v", flow.ConfigName, flow.CurrentTransition.To, args))
			workerTransitions.SetWorkerSessionContext(&workerSessionContext)
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Recovered in ProcessState for %s/%s: %v", flow.ConfigName, flow.CurrentTransition.To, r)
					logger.Debugf("Stack trace: %s", string(debug.Stack()))
				}
			}()
			workerSessionContext.Worker.SetLastResponse(workerSessionContext.Worker.GetWorkerResponse(), workerSessionContext.Worker.GetStatusCode())
			results, err = CallTransitionByName(callPath, &workerSessionContext, workerTransitions, boot.MetaFunctionCache)
			if err != nil {
				workerSessionContext.Worker.SetError(&issues.Issue{
					Message: fmt.Sprintf("Error calling transition %s: %v", callPath, err),
					Errors:  []error{err},
					Json: map[string]interface{}{
						"flow.ConfigName":          flow.ConfigName,
						"flow.CurrentTransitionTo": flow.CurrentTransition.To,
						"trace":                    flow.GetTrace(),
					},
				})
				if flow.CurrentTransition.False != "" {
					nextStateNameOrError = flow.CurrentTransition.False
				} else {
					logger.Panicf("Error calling transition %s: %v", callPath, err)
					return false, ""
				}
			}

			var result domain.FlowStepResult
			var isResultFound = false

			for _, r := range results {
				if _, ok := r.Interface().(domain.FlowStepResult); ok {
					result = r.Interface().(domain.FlowStepResult)
					isResultFound = true
					break
				}
			}

			if !isResultFound {
				workerSessionContext.Worker.SetError(&issues.Issue{
					Message: fmt.Sprintf("Please return domain.FlowStepResult in %s/%s transition with trace: %s: %v", flow.ConfigName, flow.CurrentTransition.To, flow.ConfigName, flow.GetTraceString()),
					Errors:  []error{err},
					Json: map[string]interface{}{
						"flow.ConfigName":          flow.ConfigName,
						"flow.CurrentTransitionTo": flow.CurrentTransition.To,
						"trace":                    flow.GetTrace(),
					},
				})
				if flow.CurrentTransition.False != "" {
					nextStateNameOrError = flow.CurrentTransition.False
				} else {
					logger.Errorf("Error calling transition %s: %v", callPath, err)
					return false, ""
				}
			}
			isDone = result.Success
			nextStateNameOrError = result.Next
			if result.Error != nil {
				workerSessionContext.Worker.SetError(&issues.Issue{
					Message: fmt.Sprintf("Error in %s/%s transition: %s with trace: %s: %v", flow.ConfigName, flow.CurrentTransition.To, result.Error.Error(), flow.ConfigName, flow.GetTraceString()),
					Errors:  []error{result.Error},
					Json: map[string]interface{}{
						"flow.ConfigName":          flow.ConfigName,
						"flow.CurrentTransitionTo": flow.CurrentTransition.To,
						"trace":                    flow.GetTrace(),
					},
				})
				if flow.CurrentTransition.False != "" {
					nextStateNameOrError = flow.CurrentTransition.False
				} else {
					logger.Errorf("Error in %s/%s transition: %s with trace: %s: %v", flow.ConfigName, flow.CurrentTransition.To, result.Error.Error(), flow.ConfigName, flow.GetTraceString())
					return false, ""
				}
			} else if err != nil {
				result.Error = err
			}

			if result.Response != nil {
				workerSessionContext.Worker.SetResponse(result.Response)
			}

			if result.StatusCode > 0 {
				workerSessionContext.Worker.SetStatusCode(result.StatusCode)
			}

			if result.Error != nil {
				// Preserve the status code the transition chose (404, 409, …);
				// SetError without a code resets it to 500.
				if result.StatusCode > 0 {
					workerSessionContext.Worker.SetError(issues.NewIssueFromError(result.Error), result.StatusCode)
				} else {
					workerSessionContext.Worker.SetError(issues.NewIssueFromError(result.Error))
				}
			}

			if workerSessionContext.Flow.CurrentTransition.Response != "" {
				logger.Debugf("Transition response name: %s", workerSessionContext.Flow.CurrentTransition.Response)
				workerSessionContext.SetValue(workerSessionContext.Flow.CurrentTransition.Response, result.Response)
			}

		} else {
			isDone = *flow.CurrentTransition.SkipTo
		}

		if isDone {
			// Check OnSuccessWhen condition if present
			if flow.CurrentTransition.OnSuccessWhen != nil {
				conditionProp := *flow.CurrentTransition.OnSuccessWhen
				condition, err := workerSessionContext.Parser.ParseTemplate(conditionProp)
				if !workerSessionContext.Worker.HandleError(err, http.StatusInternalServerError) {
					return false, ""
				}
				// Evaluate the condition - only "false" (as string) means condition failed
				// This matches the behavior of the existing If condition evaluation (line 230)
				if condition == "false" {
					// Condition failed, route to False path
					if flow.CurrentTransition.False != "" {
						nextStateNameOrError = flow.CurrentTransition.False
						// Keep isDone = true since we're transitioning to False path
					} else {
						// No False path defined, mark as failed
						isDone = false
					}
				} else {
					// Condition passed (true or any truthy value), route to True path
					if flow.CurrentTransition.True != "" {
						nextStateNameOrError = flow.CurrentTransition.True
					}
				}
			} else if flow.CurrentTransition.True != "" {
				// No OnSuccessWhen condition, use regular True path
				nextStateNameOrError = flow.CurrentTransition.True
			}
		} else {
			if flow.CurrentTransition.False != "" {
				nextStateNameOrError = flow.CurrentTransition.False
				isDone = true
			}
		}

		if baseWorker.Debug {
			logger.Debug(fmt.Sprintf("Method Result: isDone: %t, nextStateNameOrError: %s with trace: %s: %v", isDone, nextStateNameOrError, flow.ConfigName, flow.GetTraceString()))
		}

		workerSessionContext.SetValue("workflow.Last.Success", isDone)
		workerSessionContext.SetValue("workflow.Last.nextStateNameOrError", nextStateNameOrError)
		return isDone, nextStateNameOrError
	} else {
		nextStateNameOrError = fmt.Sprintf("Method %s not found in %s transitions with trace: %s: %v", methodName, transitionsName, flow.ConfigName, flow.GetTraceString())
		if baseWorker.Debug {
			logger.Debug(fmt.Sprintf("Method Not Found: %s, with trace: %s: %v", nextStateNameOrError, flow.ConfigName, flow.GetTraceString()))
		}
	}

	return false, nextStateNameOrError
}

//goland:noinspection GoUnusedParameter
func (baseWorker *workflowWorker) Done(w EngineInterface) *WorkerResponse {
	return baseWorker.Response
}

func (baseWorker *workflowWorker) ProcessStateError(w EngineInterface, flow *domain.Flow, response *WorkerResponse) {
	if w.isPreventedLogError() {
		return
	}

	last := flow.LastTrace
	trace := flow.Trace
	var msg string
	if baseWorker.Response.Error == nil && response.Error.Error() == "" {
		// msg = fmt.Sprintf("State returned %s FALSE on SUCCESS from %s -> %s and can't found FALSE transition for %s of %s", flow.CurrentState.State, flow.CurrentTransition.From, flow.CurrentState.State, flow.Name, flow.ConfigName)
		msg = fmt.Sprintf("Can't find FALSE transition for %s -> %s -> %s", flow.ConfigName, flow.CurrentTransition.From, flow.CurrentState.State)
	} else {
		msg = fmt.Sprintf("Can't process %s of %s from %s -> %s, with message %s %s", flow.Name, flow.ConfigName, flow.CurrentTransition.From, flow.CurrentState.State, response.Error.Error(), baseWorker.Response.Error)
	}
	o := make(map[string]interface{})
	context := baseWorker.WorkflowContext.Context()
	o[flow.Name] = map[string]interface{}{
		"error": map[string]interface{}{
			"message":       msg,
			"error_message": response.Error.Error(),
			"flow_name":     flow.Name,
			"transition":    flow.CurrentTransition.To,
			"config_name":   flow.ConfigName,
			"response":      response.Response,
		},
		"current_transition": map[string]interface{}{
			"from": flow.CurrentTransition.From,
			"to":   flow.CurrentTransition.To,
		},
		"current_state": map[string]interface{}{
			"state": flow.CurrentState.State,
		},
		"trace":   trace,
		"last":    last,
		"options": (*context)["options"],
		"values":  (*context)["values"],
	}
	baseWorker.Response.RiseAnIssueFromString(msg, o)
}

func (baseWorker *workflowWorker) SetResponse(response any, statusCode ...int) {
	baseWorker.LastResponse.Response = baseWorker.Response.Response
	baseWorker.WorkflowContext.SetValue("@", baseWorker.LastResponse.Response)
	baseWorker.Response.Response = response
	if len(statusCode) > 0 {
		baseWorker.SetStatusCode(statusCode[0])
	} else if baseWorker.GetStatusCode() == 0 {
		baseWorker.SetStatusCode(http.StatusOK)
	}
}

func (baseWorker *workflowWorker) SetLastResponse(response any, statusCode int) {
	baseWorker.LastResponse.Response = response
	baseWorker.WorkflowContext.SetValue("@", baseWorker.LastResponse.Response)
	baseWorker.SetLastStatusCode(statusCode)
}

func (baseWorker *workflowWorker) SetLastStatusCode(statusCode int) {
	baseWorker.LastResponse.StatusCode = statusCode
	baseWorker.WorkflowContext.SetValue("?", baseWorker.LastResponse.StatusCode)
}

func (baseWorker *workflowWorker) GetResponse() (response any) {
	return baseWorker.Response.Response
}

func (baseWorker *workflowWorker) GetLastResponse() (response any) {
	return baseWorker.LastResponse.Response
}

func (baseWorker *workflowWorker) GetWorkerResponse() (response *WorkerResponse) {
	return baseWorker.Response
}

func (baseWorker *workflowWorker) GetWorkerLastResponse() (response *WorkerResponse) {
	return baseWorker.Response
}

func (baseWorker *workflowWorker) HandleError(err interface{}, statusCode int) (success bool) {
	success = true
	if !helpers.IsNil(err) {
		switch err.(type) {
		case []*issues.Issue:
			for _, e := range err.([]*issues.Issue) {
				baseWorker.SetError(e, statusCode)
			}
		case *issues.Issues:
			baseWorker.SetErrors(err.(*issues.Issues), statusCode)
		case error:
			baseWorker.SetError(issues.NewIssue(err.(error), err.(error)), statusCode)
		case string:
			baseWorker.SetError(issues.NewIssue(fmt.Sprintf("%s", err), err.(error)), statusCode)
		case int:
			baseWorker.SetError(issues.NewIssue(fmt.Sprintf("%d", err), err.(error)), statusCode)
		default:
			baseWorker.SetError(issues.NewIssue(fmt.Sprintf("%v", err), err.(error)), statusCode)
		}
		success = false
	}

	return success
}

func (baseWorker *workflowWorker) SetError(error *issues.Issue, statusCode ...int) bool {
	var errorObject interface{}
	var errorStatusCode int
	if baseWorker.Response.Error == nil {
		errorObject = error
		baseWorker.LastResponse.Error = baseWorker.Response.Error
		baseWorker.WorkflowContext.SetValue("^", baseWorker.LastResponse.Error)
		baseWorker.Response.Error = issues.NewIssues(error)
	} else {
		errorObject = error
		baseWorker.LastResponse.Error = baseWorker.Response.Error
		baseWorker.WorkflowContext.SetValue("^", baseWorker.LastResponse.Error)
		baseWorker.Response.Error.Another(error)
	}
	if len(statusCode) > 0 {
		errorStatusCode = statusCode[0]
		baseWorker.SetStatusCode(statusCode[0])
	} else {
		errorStatusCode = http.StatusInternalServerError
		baseWorker.SetStatusCode(http.StatusInternalServerError)
	}

	logger.Debugf("Setting error: %s with status code: %d", localHelpers.PrintFirstLevelString(errorObject), errorStatusCode)

	return false
}

func (baseWorker *workflowWorker) SetErrors(errors *issues.Issues, statusCode ...int) bool {
	if baseWorker.Response.Error == nil {
		baseWorker.LastResponse.Error = baseWorker.Response.Error
		baseWorker.WorkflowContext.SetValue("^", baseWorker.LastResponse.Error)
		baseWorker.Response.Error = issues.NewIssues(errors.Issues...)
	} else {
		baseWorker.LastResponse.Error = baseWorker.Response.Error
		baseWorker.WorkflowContext.SetValue("^", baseWorker.LastResponse.Error)
		baseWorker.Response.Error.More(errors.Issues...)
	}
	if len(statusCode) > 0 {
		baseWorker.SetStatusCode(statusCode[0])
	} else {
		baseWorker.SetStatusCode(http.StatusInternalServerError)
	}

	return false
}

func (baseWorker *workflowWorker) MergeIssues(errors *issues.Issues, statusCode ...int) bool {
	if baseWorker.Response.Error == nil {
		baseWorker.LastResponse.Error = baseWorker.Response.Error
		baseWorker.WorkflowContext.SetValue("^", baseWorker.LastResponse.Error)
		baseWorker.Response.Error = issues.NewIssues(errors.Issues...)
	} else {
		baseWorker.LastResponse.Error = baseWorker.Response.Error
		baseWorker.WorkflowContext.SetValue("^", baseWorker.LastResponse.Error)
		baseWorker.Response.Error.More(errors.Issues...)
	}
	if len(statusCode) > 0 {
		baseWorker.SetStatusCode(statusCode[0])
	} else {
		baseWorker.SetStatusCode(http.StatusInternalServerError)
	}

	return false
}

func (baseWorker *workflowWorker) GetError() (error *issues.Issues) {
	return baseWorker.Response.Error
}

func (baseWorker *workflowWorker) SetStatusCode(statusCode int) {
	baseWorker.Response.StatusCode = statusCode
}

func (baseWorker *workflowWorker) GetStatusCode() int {
	return baseWorker.Response.StatusCode
}

func (baseWorker *workflowWorker) SetDebug(debug bool) {
	baseWorker.Debug = debug
}

func (baseWorker *workflowWorker) IsDebug() bool {
	return baseWorker.Debug
}

func (baseWorker *workflowWorker) GetWorkflowContext() WorkerContext {
	return baseWorker.WorkflowContext
}

func (baseWorker *workflowWorker) CleanErrors() bool {
	baseWorker.Response.Error = nil
	return true
}
