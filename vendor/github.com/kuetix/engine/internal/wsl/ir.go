package wsl

// BuildGraph builds a Graph from a Workflow AST.
func BuildGraph(wf Workflow) *Graph {
	g := &Graph{WorkflowName: wf.Name, WorkflowType: wf.Type, Nodes: map[string]*Node{}, Start: wf.Start}
	// create nodes
	for name, st := range wf.States {
		n := &Node{Name: name}
		if st.Start {
			n.Start = true
		}
		if st.Action != nil {
			// copy action
			a := *st.Action
			n.Action = &a
		}
		// state params
		if len(st.Params) > 0 {
			n.ParamNames = append(n.ParamNames, st.Params...)
		}
		// Copy new state attributes
		if st.IfExpr != nil {
			n.IfExpr = &Expr{Raw: st.IfExpr.Raw}
		}
		n.ContinueOnFail = st.ContinueOnFail
		n.SkipTo = st.SkipTo
		n.Parallel = st.Parallel
		n.ParallelCount = st.ParallelCount
		n.Wait = st.Wait
		n.JoinTarget = st.JoinTarget
		if st.End != nil {
			n.Terminal = true
			n.TerminalKind = st.End.Kind
			n.Attr = map[string]string{}
			for k, v := range st.End.Attr {
				n.Attr[k] = v
			}
		}
		// edges
		for _, tr := range st.Transitions {
			edge := Edge{Condition: tr.Condition, To: tr.Target}
			if tr.Start {
				n.Start = true
			}
			if tr.WhenExpr != nil {
				edge.WhenExpr = &Expr{Raw: tr.WhenExpr.Raw}
			}
			if len(tr.Args) > 0 {
				edge.Args = append(edge.Args, tr.Args...)
			}
			n.Edges = append(n.Edges, edge)
		}
		g.Nodes[name] = n
	}
	return g
}

// BuildGraphs builds graphs for all workflows in the module.
func BuildGraphs(m *Module) map[string]*Graph {
	res := map[string]*Graph{}
	// prepare constants map once from module
	consts := map[string]interface{}{}
	if len(m.Constants) > 0 {
		for _, c := range m.Constants {
			consts[c.Name] = c.Value
		}
	}
	for _, wf := range m.Workflows {
		g := BuildGraph(wf)
		if len(consts) > 0 {
			// assign a copy to avoid accidental mutation across graphs
			g.Constants = map[string]interface{}{}
			for k, v := range consts {
				g.Constants[k] = v
			}
		}
		res[wf.Name] = g
	}
	return res
}
