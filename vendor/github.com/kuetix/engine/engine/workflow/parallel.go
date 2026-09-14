package workflow

import (
	"fmt"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/issues"
	"github.com/kuetix/logger"
)

// parallelGroupKeyPrefix namespaces in-flight parallel groups inside the
// workflow context so wait states can find them by the parallel state's name.
const parallelGroupKeyPrefix = "workflow.parallel."

// parallelBranchResult holds the outcome of one branch of a parallel state.
type parallelBranchResult struct {
	Index    int
	Success  bool
	Error    error
	Response interface{}
}

// parallelGroup tracks the in-flight branches of one `parallel[count: N]`
// state between fork and join. All branches always run to completion; the
// joining wait state reports an aggregated failure if any branch failed.
type parallelGroup struct {
	Name          string
	Count         int
	ResponseAlias string
	wg            sync.WaitGroup
	results       []parallelBranchResult
	// sharedMu serializes branches that fall back to a shared transition
	// instance (when DI cannot mint a fresh one): SetSession stores the
	// session on the instance itself, so sharing is unsafe concurrently.
	sharedMu sync.Mutex
}

// processParallelFork spawns Count concurrent executions of the parallel
// state's action and returns immediately so the main flow can continue.
// Branches are joined later by the matching wait state.
func (baseWorker *workflowWorker) processParallelFork(session *WorkerSessionContext) (bool, string) {
	flow := session.Flow
	count := flow.CurrentTransition.ParallelCount
	groupName := strings.SplitN(flow.CurrentTransition.Name, "#", 2)[0]
	callPath := strings.Split(flow.CurrentState.State, "#")[0]

	resolverKey, workerTransitions := baseWorker.resolveTransitionsForPath(callPath)
	if workerTransitions == nil {
		baseWorker.SetError(&issues.Issue{
			Message: fmt.Sprintf("parallel state '%s': transitions not found for %s, add its module to resolvers", groupName, callPath),
		}, http.StatusInternalServerError)
		return false, ""
	}

	group := &parallelGroup{
		Name:          groupName,
		Count:         count,
		ResponseAlias: flow.CurrentTransition.Response,
		results:       make([]parallelBranchResult, count),
	}
	group.wg.Add(count)

	logger.Debugf("[parallel] %s: forking %d branches of %s", groupName, count, callPath)

	for i := 0; i < count; i++ {
		// Snapshot the context and flow synchronously so branches are isolated
		// from the main flow, which keeps stepping while they run.
		branchCtx := make(map[string]interface{}, len(*baseWorker.WorkflowContext.Context())+1)
		for k, v := range *baseWorker.WorkflowContext.Context() {
			branchCtx[k] = v
		}
		branchCtx["branch"] = map[string]interface{}{"index": i, "count": count}
		branchContext := NewWorkflowContext(&branchCtx)

		branchWorker := &workflowWorker{
			WorkflowContext:       branchContext,
			Transitions:           baseWorker.Transitions,
			TransitionsResolved:   baseWorker.TransitionsResolved,
			TransitionsNamespaces: baseWorker.TransitionsNamespaces,
			Response:              &WorkerResponse{},
			LastResponse:          &WorkerResponse{},
			Debug:                 baseWorker.Debug,
			Options:               baseWorker.Options,
		}

		branchFlow := *flow // pin CurrentState/CurrentTransition at fork time
		branchSession := &WorkerSessionContext{
			Engine:          session.Engine,
			Flow:            &branchFlow,
			Worker:          branchWorker,
			WorkflowContext: branchContext,
			ServerContext:   session.ServerContext,
			Parser:          NewParser(),
		}
		branchCtx["workflow.Flow"] = &branchFlow
		branchCtx["workflow.Worker"] = branchWorker

		go runParallelBranch(group, i, callPath, resolverKey, workerTransitions, branchSession)
	}

	baseWorker.WorkflowContext.SetValue(parallelGroupKeyPrefix+groupName, group)

	if flow.CurrentTransition.True != "" {
		return true, flow.CurrentTransition.True
	}
	return true, ""
}

// runParallelBranch executes one branch of a parallel group and records its
// result. Each transition instance is resolved fresh from DI where possible so
// branches do not share the transition's session state.
func runParallelBranch(group *parallelGroup, index int, callPath string, resolverKey string, shared *ServiceTransitionMapping, branchSession *WorkerSessionContext) {
	defer group.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[parallel] %s branch %d: recovered: %v", group.Name, index, r)
			logger.Debugf("Stack trace: %s", string(debug.Stack()))
			group.results[index] = parallelBranchResult{Index: index, Success: false, Error: fmt.Errorf("panic in branch %d: %v", index, r)}
		}
	}()

	mapping := shared
	if resolverKey != "" && di.CanResolve(defines.TransitionPrefix+resolverKey) {
		fresh := di.Resolve(defines.TransitionPrefix + resolverKey).(ServiceTransitionMapping)
		mapping = &fresh
	} else {
		group.sharedMu.Lock()
		defer group.sharedMu.Unlock()
	}
	mapping.SetWorkerSessionContext(branchSession)

	results, err := CallTransitionByName(callPath, branchSession, mapping, boot.MetaFunctionCache)
	if err != nil {
		group.results[index] = parallelBranchResult{Index: index, Success: false, Error: err}
		return
	}

	var stepResult domain.FlowStepResult
	found := false
	for _, r := range results {
		if sr, ok := r.Interface().(domain.FlowStepResult); ok {
			stepResult = sr
			found = true
			break
		}
	}
	if !found {
		group.results[index] = parallelBranchResult{Index: index, Success: false, Error: fmt.Errorf("transition %s did not return domain.FlowStepResult", callPath)}
		return
	}

	group.results[index] = parallelBranchResult{
		Index:    index,
		Success:  stepResult.Success && stepResult.Error == nil,
		Error:    stepResult.Error,
		Response: stepResult.Response,
	}
}

// processParallelWait blocks until every branch of the joined parallel group
// has finished, then aggregates results: on full success the wait state's
// success path is taken and the parallel state's alias (if any) is bound to
// the ordered array of branch responses; if any branch failed, the error path
// is taken with an aggregated issue.
func (baseWorker *workflowWorker) processParallelWait(session *WorkerSessionContext) (bool, string) {
	flow := session.Flow
	joinName := flow.CurrentTransition.WaitJoin
	key := parallelGroupKeyPrefix + joinName

	raw := baseWorker.WorkflowContext.Value(key)
	if raw == nil {
		baseWorker.SetError(&issues.Issue{
			Message: fmt.Sprintf("wait state '%s' joins '%s' but no parallel group was started", flow.CurrentTransition.Name, joinName),
		}, http.StatusInternalServerError)
		return false, ""
	}
	group, ok := raw.(*parallelGroup)
	if !ok {
		baseWorker.SetError(&issues.Issue{
			Message: fmt.Sprintf("wait state '%s': context key %s does not hold a parallel group", flow.CurrentTransition.Name, key),
		}, http.StatusInternalServerError)
		return false, ""
	}

	logger.Debugf("[parallel] %s: waiting for %d branches", joinName, group.Count)
	group.wg.Wait()
	baseWorker.WorkflowContext.SetValue(key, nil)

	responses := make([]interface{}, group.Count)
	var failed []parallelBranchResult
	for _, res := range group.results {
		responses[res.Index] = res.Response
		if !res.Success {
			failed = append(failed, res)
		}
	}

	if group.ResponseAlias != "" {
		baseWorker.WorkflowContext.SetValue(group.ResponseAlias, responses)
	}

	if len(failed) > 0 {
		var errs []error
		msgs := make([]string, 0, len(failed))
		for _, f := range failed {
			msgs = append(msgs, fmt.Sprintf("branch %d: %v", f.Index, f.Error))
			if f.Error != nil {
				errs = append(errs, f.Error)
			}
		}
		baseWorker.SetError(&issues.Issue{
			Message: fmt.Sprintf("parallel '%s': %d of %d branches failed: %s", joinName, len(failed), group.Count, strings.Join(msgs, "; ")),
			Errors:  errs,
			Json: map[string]interface{}{
				"parallel":        joinName,
				"failed":          len(failed),
				"count":           group.Count,
				"failures":        msgs,
				"flow.ConfigName": flow.ConfigName,
			},
		}, http.StatusInternalServerError)
		if flow.CurrentTransition.False != "" {
			return true, flow.CurrentTransition.False
		}
		return false, ""
	}

	baseWorker.SetResponse(responses)
	if flow.CurrentTransition.True != "" {
		return true, flow.CurrentTransition.True
	}
	return true, ""
}

// resolveTransitionsForPath resolves the ServiceTransitionMapping for a call
// path using the same lookup rules as ProcessState, and also returns the
// resolver key it was registered under (for fresh per-branch DI resolution).
func (baseWorker *workflowWorker) resolveTransitionsForPath(callPath string) (string, *ServiceTransitionMapping) {
	path := strings.Split(callPath, "/")
	transitionsName := strings.Join(path[:len(path)-1], "/")
	if transitionsName == "" {
		for tn, wt := range baseWorker.TransitionsResolved {
			return tn, wt
		}
		return "", nil
	}
	if wt, ok := baseWorker.TransitionsResolved[transitionsName]; ok {
		return transitionsName, wt
	}
	for _, ns := range baseWorker.TransitionsNamespaces {
		tnNs := filepath.ToSlash(filepath.Join(ns, transitionsName))
		if wt, ok := baseWorker.TransitionsResolved[tnNs]; ok {
			return tnNs, wt
		}
	}
	return "", nil
}
