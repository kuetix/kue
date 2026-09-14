package workflow

import (
	"fmt"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/internal/wsl"
)

// wslGraphToSchema converts a WSL graph to an engine Flow-compatible schema map.
// The schema matches the JSON structure expected by domain.Flow.FromMap.
func wslGraphToSchema(g *wsl.Graph) map[string]interface{} {
	// Build resolvers from action modules used in nodes
	resolversSet := map[string]struct{}{}

	// Collect nodes and build predecessors map and incoming args map
	predecessors := map[string][]string{}
	incomingArgs := map[string]map[string][]string{} // target -> from(display) -> args(raw)
	nodeByName := map[string]*wsl.Node{}
	for name, n := range g.Nodes {
		nodeByName[name] = n
		if n.Action != nil {
			mod := n.Action.Module
			if mod != "" {
				resolversSet[mod] = struct{}{}
			}
		}
		// init predecessors slice
		predecessors[name] = []string{}
	}

	for fromName, n := range g.Nodes {
		for _, e := range n.Edges {
			// e.To is target node name
			fromDisplay := stateName(nodeByName, fromName)
			if _, ok := predecessors[e.To]; ok {
				predecessors[e.To] = append(predecessors[e.To], fromDisplay)
			} else {
				predecessors[e.To] = []string{fromDisplay}
			}
			if len(e.Args) > 0 {
				if _, ok := incomingArgs[e.To]; !ok {
					incomingArgs[e.To] = map[string][]string{}
				}
				arr := make([]string, 0, len(e.Args))
				for _, a := range e.Args {
					arr = append(arr, strings.TrimSpace(a.Raw))
				}
				incomingArgs[e.To][fromDisplay] = arr
			}
		}
	}

	// Build a states array with initial/final kinds
	states := make([]map[string]interface{}, 0, len(g.Nodes))
	for name, n := range g.Nodes {
		st := map[string]interface{}{
			"name":  name,
			"state": stateName(nodeByName, name),
			"type":  domain.StateNormal,
			"node":  n,
		}
		// attach constants to state options if present
		if g.Constants != nil && len(g.Constants) > 0 {
			constMap := map[string]interface{}{}
			for k, v := range g.Constants {
				constMap[k] = v
			}
			st["constants"] = constMap
		}
		if len(n.ParamNames) > 0 {
			params := make([]string, 0, len(n.ParamNames))
			params = append(params, n.ParamNames...)
			st["params"] = params
		}
		if n.Action != nil && n.Action.As != "" {
			st["response"] = n.Action.As
		}
		st["start"] = false
		if name == g.Start {
			st["type"] = domain.StateInitial
			st["start"] = true
		}
		if n.Terminal {
			st["type"] = domain.StateFinal
			st["final_kind"] = n.TerminalKind
		}
		states = append(states, st)
	}

	// Build transitions: one per node (state) so that branching (true/false/else) is set
	transitions := make([]map[string]interface{}, 0, len(g.Nodes))

	// helper to map outgoing edges of a node to True/False/Else/OnSuccessWhen
	edgeMap := func(n *wsl.Node) (trueNext, falseNext string, elseNext *string, onSuccessWhen *string) {
		var tNext string
		var fNext string
		var eNext *string
		var oswExpr *string
		for _, e := range n.Edges {
			switch e.Condition.Kind {
			case wsl.CondSuccess:
				// Check if this success edge has a when expression
				if e.WhenExpr != nil && e.WhenExpr.Raw != "" {
					// This is "on success when <condition>" - map to on_success_when
					if oswExpr == nil {
						// Use the first on_success_when condition found
						oswExpr = &e.WhenExpr.Raw
						// The target becomes the True path
						if tNext == "" {
							tNext = stateName(nodeByName, e.To)
						}
					}
					// Note: Multiple "on success when" conditions in WSL will need
					// to be evaluated sequentially. For now, we capture the first one.
				} else {
					// Regular "on success" without condition
					if tNext == "" {
						tNext = stateName(nodeByName, e.To)
					}
				}
			case wsl.CondError:
				if fNext == "" {
					fNext = stateName(nodeByName, e.To)
				}
			case wsl.CondElse:
				v := stateName(nodeByName, e.To)
				eNext = &v
			case wsl.CondExpr:
				// MVP: treat as Else to preserve a path if provided, since engine doesn't yet post-action expr-branch
				if eNext == nil {
					v := stateName(nodeByName, e.To)
					eNext = &v
				}
			}
		}
		return tNext, fNext, eNext, oswExpr
	}

	// Ensure there is a synthetic first transition from "_" into start
	if node, ok := g.Nodes[g.Start]; ok {
		toName := stateName(nodeByName, g.Start)
		n := g.Nodes[g.Start]
		tNext, fNext, eNext, oswExpr := edgeMap(n)
		tr := map[string]interface{}{
			"name": node.Name,
			"to":   toName,
			"from": []string{"_"},
			"node": n,
		}
		// attach constants to transition options if present
		if g.Constants != nil && len(g.Constants) > 0 {
			constMap := map[string]interface{}{}
			for k, v := range g.Constants {
				constMap[k] = v
			}
			tr["constants"] = constMap
		}
		if tNext != "" {
			tr["true"] = tNext
		}
		if fNext != "" {
			tr["false"] = fNext
		}
		if eNext != nil {
			tr["else"] = *eNext
		}
		if oswExpr != nil {
			tr["on_success_when"] = *oswExpr
		}
		// Add node-level attributes to transition
		if n.IfExpr != nil {
			tr["if"] = n.IfExpr.Raw
		}
		if n.ContinueOnFail {
			tr["continue_on_fail"] = true
		}
		if n.SkipTo {
			tr["skipTo"] = true
		}
		if n.Parallel {
			tr["parallel_count"] = n.ParallelCount
		}
		if n.Wait {
			tr["wait_join"] = n.JoinTarget
		}
		// inject action args as options for this transition
		if n.Action != nil {
			mergeActionArgsIntoTransition(tr, argsToOptions(n.Action.Args))
			if n.Action != nil && n.Action.As != "" {
				tr["response"] = n.Action.As
			}
		}
		// pass target state param names for potential binding
		if len(n.ParamNames) > 0 {
			params := make([]string, 0, len(n.ParamNames))
			params = append(params, n.ParamNames...)
			tr["_call.paramNames"] = params
		}

		if n.Terminal {
			tr["type"] = domain.StateFinal
			tr["final_kind"] = n.TerminalKind
		}
		tr["start"] = false
		if n.Start {
			tr["start"] = true
		}

		transitions = append(transitions, tr)
	}

	// For every non-start node, create transition with From = predecessors
	for name, n := range g.Nodes {
		if name == g.Start {
			continue
		}
		toName := stateName(nodeByName, name)
		fromList := predecessors[name]
		if len(fromList) == 0 {
			// default from previous (engine will fix), but keep empty slice to avoid nil so CorrectFlow won't auto-wire wrong
			fromList = []string{"_"}
		}
		tNext, fNext, eNext, oswExpr := edgeMap(n)
		tr := map[string]interface{}{
			"name": name,
			"to":   toName,
			"from": fromList,
			"node": n,
		}
		if tNext != "" {
			tr["true"] = tNext
		}
		if fNext != "" {
			tr["false"] = fNext
		}
		if eNext != nil {
			tr["else"] = *eNext
		}
		if oswExpr != nil {
			tr["on_success_when"] = *oswExpr
		}
		// Add node-level attributes to transition
		if n.IfExpr != nil {
			tr["if"] = n.IfExpr.Raw
		}
		if n.ContinueOnFail {
			tr["continue_on_fail"] = true
		}
		if n.SkipTo {
			tr["skipTo"] = true
		}
		if n.Parallel {
			tr["parallel_count"] = n.ParallelCount
		}
		if n.Wait {
			tr["wait_join"] = n.JoinTarget
		}
		// inject action args as options for this transition
		if n.Action != nil {
			mergeActionArgsIntoTransition(tr, argsToOptions(n.Action.Args))
			if n.Action != nil && n.Action.As != "" {
				tr["response"] = n.Action.As
			}
		}
		// pass target state param names and incoming call args mapping
		if len(n.ParamNames) > 0 {
			params := make([]string, 0, len(n.ParamNames))
			params = append(params, n.ParamNames...)
			tr["_call.paramNames"] = params
		}
		if amap, ok := incomingArgs[name]; ok && len(amap) > 0 {
			tr["_call.args.map"] = amap
		}
		if n.Terminal {
			tr["type"] = domain.StateFinal
			tr["final_kind"] = n.TerminalKind
		}
		tr["start"] = false
		if n.Start {
			tr["start"] = true
		}

		transitions = append(transitions, tr)
	}

	// Extract resolvers
	resolvers := make([]string, 0, len(resolversSet))
	for r := range resolversSet {
		resolvers = append(resolvers, r)
	}

	schema := map[string]interface{}{
		"type":        g.WorkflowType, // Include workflow type
		"resolvers":   resolvers,
		"states":      states,
		"transitions": transitions,
	}

	return schema
}

// reservedTransitionKeys are keys set by the schema builder on transition maps.
// Action args matching these names would otherwise overwrite the schema fields
// (or be overwritten by them) when the transition map is decoded into FlowTransition.
var reservedTransitionKeys = map[string]struct{}{
	"name":             {},
	"to":               {},
	"from":             {},
	"node":             {},
	"constants":        {},
	"true":             {},
	"false":            {},
	"else":             {},
	"on_success_when":  {},
	"if":               {},
	"continue_on_fail": {},
	"skipTo":           {},
	"parallel_count":   {},
	"wait_join":        {},
	"response":         {},
	"_call.paramNames": {},
	"_call.args.map":   {},
	"type":             {},
	"final_kind":       {},
	"start":            {},
	"options":          {},
}

func isReservedTransitionKey(k string) bool {
	_, ok := reservedTransitionKeys[k]
	return ok
}

// mergeActionArgsIntoTransition merges action args into the transition map.
// Non-reserved arg names are placed at the top level (so existing top-level
// option lookups continue to work); reserved arg names are nested under
// `options` so they reach the engine via FlowTransition.Options["options"]
// and remain discoverable through OrderSubOptionsSearch ("options" sub-group),
// without clobbering schema fields like Name/From/To/If/...
func mergeActionArgsIntoTransition(tr map[string]interface{}, args map[string]interface{}) {
	if len(args) == 0 {
		return
	}
	var nested map[string]interface{}
	if existing, ok := tr["options"].(map[string]interface{}); ok {
		nested = existing
	}
	for k, v := range args {
		if isReservedTransitionKey(k) {
			if nested == nil {
				nested = map[string]interface{}{}
			}
			nested[k] = v
			continue
		}
		tr[k] = v
	}
	if len(nested) > 0 {
		tr["options"] = nested
	}
}

// argsToOptions converts WSL action args into engine transition options.
// Supported forms per arg: key: value where value is number, boolean, or string/ident.
func argsToOptions(args []wsl.Expr) map[string]interface{} {
	opts := map[string]interface{}{}
	mergeMap := func(m map[string]interface{}) {
		for k, v := range m {
			opts[k] = v
		}
	}
	for _, ex := range args {
		raw := strings.TrimSpace(ex.Raw)
		if raw == "" {
			continue
		}
		// If the arg is an object literal, parse and merge
		if strings.HasPrefix(raw, "{") {
			if m, ok := parseWSLObject(raw); ok {
				// Resolve placeholders to <<...>> and basic types are already parsed
				mergeMap(m)
				continue
			}
		}
		// split at first ':' for key: value pairs
		key := raw
		val := ""
		if idx := strings.Index(raw, ":"); idx >= 0 {
			key = strings.TrimSpace(raw[:idx])
			val = strings.TrimSpace(raw[idx+1:])
		} else {
			// bare value; try to parse and store as is (not common for options)
			if v, ok := parseWSLValue(raw); ok {
				// if the parsed value is a map, merge it
				if m, ok2 := v.(map[string]interface{}); ok2 {
					mergeMap(m)
				} else {
					// No key to attach; skip to avoid unnamed option
				}
			}
			continue
		}
		if key == "" {
			continue
		}
		// Parse the value using the common parser (numbers, bools, quoted strings, placeholders, objects)
		if v, ok := parseWSLValue(val); ok {
			opts[key] = v
			continue
		}
	}
	return opts
}

// stateName returns name used by engine for a WSL node. If node has Action, use module/name, else use raw node name.
func stateName(nodes map[string]*wsl.Node, name string) string {
	if n, ok := nodes[name]; ok && n.Action != nil {
		mod := n.Action.Module
		fn := n.Action.Name
		if mod == "" {
			return fn
		}
		return fmt.Sprintf("%s/%s#%s", strings.Trim(mod, "/"), fn, name)
	}
	// fallback to node label
	return name
}
