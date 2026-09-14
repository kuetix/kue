package wsl

import (
	"fmt"
	"strings"
)

// BuildAST converts a CSTModule to an AST Module with semantic validation.
func BuildAST(cst *CSTModule) (*Module, error) {
	if cst == nil {
		return nil, &SemanticError{Msg: "nil CST"}
	}
	m := &Module{Name: cst.NameTok.Lexeme}
	// extends
	if len(cst.Extends) > 0 {
		for _, e := range cst.Extends {
			val := e.ValueTok.Lexeme
			if e.ValueTok.Kind == TokString {
				val = unescapeString(val)
			}
			m.Extends = append(m.Extends, val)
		}
	}
	// imports
	for _, ci := range cst.Imports {
		imp := Import{Path: ci.PathTok.Lexeme}
		if ci.AliasTok != nil {
			imp.As = ci.AliasTok.Lexeme
		}
		m.Imports = append(m.Imports, imp)
	}
	// context
	if cst.Context != nil {
		for _, f := range cst.Context.Fields {
			m.Context = append(m.Context, Field{Name: f.NameTok.Lexeme, Type: f.TypeTok.Lexeme})
		}
	}
	// constants
	if cst.Constants != nil {
		for _, e := range cst.Constants.Entries {
			val, err := convertConstValue(e.Val)
			if err != nil {
				return nil, &SemanticError{Msg: fmt.Sprintf("error converting constant '%s': %v", e.Key.Lexeme, err)}
			}
			m.Constants = append(m.Constants, Constant{Name: e.Key.Lexeme, Value: val})
		}
	}
	// workflows
	for _, cw := range cst.Workflows {
		// Extract the workflow type from TypeTok
		workflowType := cw.TypeTok.Lexeme
		wf := Workflow{Name: cw.NameTok.Lexeme, Type: workflowType, Start: cw.StartName.Lexeme, States: map[string]*State{}}
		for idx, cs := range cw.States {
			st := &State{Name: cs.NameTok.Lexeme}
			// state parameters
			if len(cs.Params) > 0 {
				for _, pt := range cs.Params {
					st.Params = append(st.Params, pt.Lexeme)
				}
			}
			// state attributes
			if cs.IfExpr != nil {
				st.IfExpr = &Expr{Raw: cs.IfExpr.Raw}
			}
			st.ContinueOnFail = cs.ContinueOnFail
			st.SkipTo = cs.SkipTo
			if cs.Parallel {
				st.Parallel = true
				count, err := parallelCount(cs.ParallelAttrs)
				if err != nil {
					return nil, &SemanticError{Msg: fmt.Sprintf("state '%s' in workflow '%s': %v", st.Name, wf.Name, err)}
				}
				st.ParallelCount = count
			}
			if cs.Wait {
				st.Wait = true
				if cs.JoinTarget == nil {
					return nil, &SemanticError{Msg: fmt.Sprintf("wait state '%s' in workflow '%s' must declare 'join <ParallelState>'", st.Name, wf.Name)}
				}
				st.JoinTarget = cs.JoinTarget.Lexeme
			}
			if cs.Action != nil {
				st.Action = toAction(cs.Action)
			}
			for _, ct := range cs.Transitions {
				cond := toCondition(ct.Cond)
				target := ct.TargetTok.Lexeme
				if target == "_" {
					if idx == len(cw.States)-1 {
						// Trailing "on success -> _" means this state is terminal success.
						if cond.Kind == CondSuccess {
							if st.End == nil {
								st.End = &End{Kind: "ok", Attr: map[string]string{}}
							}
							continue
						}
						return nil, &SemanticError{Msg: fmt.Sprintf("state '%s' in workflow '%s' uses '_' transition target with no following state", st.Name, wf.Name)}
					}
					target = cw.States[idx+1].NameTok.Lexeme
				}
				tr := Transition{Name: cs.NameTok.Lexeme, Condition: cond, Target: target, Start: cw.StartName.Lexeme == cs.NameTok.Lexeme}
				// when expression if present
				if ct.Cond.WhenExpr != nil {
					tr.WhenExpr = &Expr{Raw: ct.Cond.WhenExpr.Raw}
				}
				// transition call args
				if len(ct.Args) > 0 {
					for _, a := range ct.Args {
						tr.Args = append(tr.Args, Expr{Raw: a.Raw})
					}
				}
				st.Transitions = append(st.Transitions, tr)
			}
			if cs.End != nil {
				e := &End{Kind: strings.ToLower(cs.End.KindTok.Kind.String()), Attr: map[string]string{}}
				for _, a := range cs.End.Attrs {
					// strip quotes for strings
					val := a.Val.Lexeme
					if a.Val.Kind == TokString {
						val = unescapeString(val)
					}
					e.Attr[a.Key.Lexeme] = val
				}
				st.End = e
			}
			if _, exists := wf.States[st.Name]; exists {
				return nil, &SemanticError{Msg: fmt.Sprintf("duplicate state '%s' in workflow '%s'", st.Name, wf.Name)}
			}
			// validation: if end present, only non-success (error/else) transitions are allowed.
			// A success transition conflicts with 'end' since both define what happens on success.
			if st.End != nil {
				for _, tr := range st.Transitions {
					if tr.Condition.Kind == CondSuccess {
						return nil, &SemanticError{Msg: fmt.Sprintf("state '%s' in workflow '%s' has both 'end' and a success transition", st.Name, wf.Name)}
					}
				}
			}
			wf.States[st.Name] = st
		}
		// validations
		if _, ok := wf.States[wf.Start]; !ok {
			return nil, &SemanticError{Msg: fmt.Sprintf("start state '%s' not found in workflow '%s'", wf.Start, wf.Name)}
		}
		// check transition targets
		for _, st := range wf.States {
			for _, tr := range st.Transitions {
				if _, ok := wf.States[tr.Target]; !ok {
					return nil, &SemanticError{Msg: fmt.Sprintf("transition from '%s' targets unknown state '%s' in workflow '%s'", st.Name, tr.Target, wf.Name)}
				}
			}
		}
		// parallel/wait validation: every parallel state must have an action and
		// exactly one wait state joining it; every wait must join a parallel state.
		joinedBy := map[string]string{} // parallel state -> wait state
		for _, st := range wf.States {
			if st.Wait {
				target, ok := wf.States[st.JoinTarget]
				if !ok {
					return nil, &SemanticError{Msg: fmt.Sprintf("wait state '%s' joins unknown state '%s' in workflow '%s'", st.Name, st.JoinTarget, wf.Name)}
				}
				if !target.Parallel {
					return nil, &SemanticError{Msg: fmt.Sprintf("wait state '%s' joins '%s' which is not a parallel state in workflow '%s'", st.Name, st.JoinTarget, wf.Name)}
				}
				if prev, dup := joinedBy[st.JoinTarget]; dup {
					return nil, &SemanticError{Msg: fmt.Sprintf("parallel state '%s' is joined by both '%s' and '%s' in workflow '%s'", st.JoinTarget, prev, st.Name, wf.Name)}
				}
				joinedBy[st.JoinTarget] = st.Name
			}
		}
		for _, st := range wf.States {
			if st.Parallel {
				if st.Action == nil {
					return nil, &SemanticError{Msg: fmt.Sprintf("parallel state '%s' in workflow '%s' must have an action", st.Name, wf.Name)}
				}
				if _, ok := joinedBy[st.Name]; !ok {
					return nil, &SemanticError{Msg: fmt.Sprintf("parallel state '%s' in workflow '%s' has no matching 'wait' state (branches would leak)", st.Name, wf.Name)}
				}
			}
		}
		m.Workflows = append(m.Workflows, wf)
	}
	return m, nil
}

// parallelCount extracts the required integer 'count' attribute from a
// parallel state's (or SWSL fork action's) attribute list.
func parallelCount(attrs []CSTConstEntry) (int, error) {
	for _, e := range attrs {
		if e.Key.Lexeme != "count" {
			continue
		}
		val, err := convertConstValue(e.Val)
		if err != nil {
			return 0, fmt.Errorf("invalid 'count' value: %v", err)
		}
		switch v := val.(type) {
		case int64:
			if v < 1 {
				return 0, fmt.Errorf("'count' must be >= 1, got %d", v)
			}
			return int(v), nil
		default:
			return 0, fmt.Errorf("'count' must be an integer, got %T", val)
		}
	}
	return 0, fmt.Errorf("parallel state requires a 'count' attribute, e.g. parallel[count: 4]")
}

func toAction(ca *CSTAction) *Action {
	qname := ca.QNameTok.Lexeme
	var mod, name string
	// Sub-workflow actions are stored as "type name" (e.g., "workflow step1").
	// Detect them by checking for the known type prefixes so we can preserve the
	// combined string as the Name without running it through splitQName.
	if isSubWorkflowQName(qname) {
		name = qname
	} else {
		mod, name = splitQName(qname)
	}
	act := &Action{Module: mod, Name: name}
	for _, a := range ca.Args {
		act.Args = append(act.Args, Expr{Raw: a.Raw})
	}
	if ca.AliasTok != nil {
		act.As = ca.AliasTok.Lexeme
	}
	return act
}

// isSubWorkflowQName reports whether qname is a sub-workflow invocation of the
// form "workflow <name>", "feature <name>", or "solution <name>".
func isSubWorkflowQName(qname string) bool {
	for prefix := range subWorkflowKinds {
		if strings.HasPrefix(qname, prefix+" ") {
			return true
		}
	}
	return strings.HasPrefix(qname, "workflow ")
}

func toCondition(cc CSTCondition) Condition {
	switch cc.Kind {
	case TokSuccess:
		return Condition{Kind: CondSuccess}
	case TokError, TokFail:
		return Condition{Kind: CondError}
	case TokElse:
		return Condition{Kind: CondElse}
	default:
		if cc.Expr != nil {
			return Condition{Kind: CondExpr, Expr: &Expr{Raw: cc.Expr.Raw}}
		}
		return Condition{Kind: CondExpr, Expr: &Expr{Raw: ""}}
	}
}

func splitQName(q string) (module, name string) {
	if i := strings.LastIndex(q, "."); i >= 0 {
		return q[:i], q[i+1:]
	}
	return "", q
}

// unescapeString replaces common escape sequences in already-unquoted string content.
func unescapeString(s string) string {
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\r", "\r")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// convertConstValue converts a CSTValue to Go interface{}
func convertConstValue(v CSTValue) (interface{}, error) {
	switch v.Kind {
	case CSTValueScalar:
		if v.Token == nil {
			return nil, fmt.Errorf("scalar value has nil token")
		}
		return convertScalarToken(*v.Token), nil
	case CSTValueObject:
		if v.Object == nil {
			return nil, fmt.Errorf("object value has nil object")
		}
		return convertConstObject(*v.Object)
	case CSTValueArray:
		if v.Array == nil {
			return nil, fmt.Errorf("array value has nil array")
		}
		return convertConstArray(*v.Array)
	default:
		return nil, fmt.Errorf("unknown value kind")
	}
}

// convertScalarToken converts a token to its Go value
func convertScalarToken(tok Token) interface{} {
	switch tok.Kind {
	case TokString:
		return unescapeString(tok.Lexeme)
	case TokNumber:
		// Try to parse as int first, then float
		if strings.Contains(tok.Lexeme, ".") {
			// Parse as float
			var f float64
			_, err := fmt.Sscanf(tok.Lexeme, "%f", &f)
			if err != nil {
				return nil
			}
			return f
		}
		// Parse as int
		var i int64
		_, err := fmt.Sscanf(tok.Lexeme, "%d", &i)
		if err != nil {
			return nil
		}
		return i
	case TokIdent:
		// Handle boolean literals
		lex := tok.Lexeme
		if lex == "true" {
			return true
		}
		if lex == "false" {
			return false
		}
		if lex == "null" {
			return nil
		}
		// Return as string for other identifiers
		return lex
	default:
		return tok.Lexeme
	}
}

// convertConstObject converts a CSTObject to map[string]interface{}
func convertConstObject(obj CSTObject) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for _, entry := range obj.Entries {
		key := entry.Key.Lexeme
		if entry.Key.Kind == TokString {
			key = unescapeString(key)
		}
		val, err := convertConstValue(entry.Val)
		if err != nil {
			return nil, fmt.Errorf("error converting object entry '%s': %v", key, err)
		}
		result[key] = val
	}
	return result, nil
}

// convertConstArray converts a CSTArray to []interface{}
func convertConstArray(arr CSTArray) ([]interface{}, error) {
	result := make([]interface{}, 0, len(arr.Values))
	for i, val := range arr.Values {
		v, err := convertConstValue(val)
		if err != nil {
			return nil, fmt.Errorf("error converting array element %d: %v", i, err)
		}
		result = append(result, v)
	}
	return result, nil
}
