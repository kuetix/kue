package wsl

// AST semantic model

type Module struct {
	Name      string
	Imports   []Import
	Extends   []string
	Context   []Field
	Constants []Constant
	Workflows []Workflow
}

type Import struct {
	Path string
	As   string // optional alias
}

type Field struct {
	Name string
	Type string
}

type Constant struct {
	Name  string
	Value interface{} // can be string, number, bool, map[string]interface{}, []interface{}
}

type Workflow struct {
	Name   string
	Type   string // workflow type (workflow, feature, solution, etc.)
	Start  string
	States map[string]*State
}

type State struct {
	Name           string
	Params         []string
	Action         *Action
	Transitions    []Transition
	Start          bool
	End            *End
	IfExpr         *Expr // optional if condition expression
	ContinueOnFail bool  // continue on fail flag
	SkipTo         bool  // skip to flag
	// Parallel fork: run the state's action ParallelCount times concurrently.
	Parallel      bool
	ParallelCount int
	// Wait (join) state: block until all branches of JoinTarget finish.
	Wait       bool
	JoinTarget string
}

type Action struct {
	Module string // left part of qname if any
	Name   string // right part of qname
	Args   []Expr
	As     string // alias for result
}

type Transition struct {
	Name      string
	Condition Condition
	Start     bool
	WhenExpr  *Expr // optional when condition expression
	Target    string
	Args      []Expr
}

type End struct {
	Kind string            // "ok" or "fail"
	Attr map[string]string // attributes from end statement
}

// Condition kind
const (
	CondSuccess = "success"
	CondError   = "error"
	CondElse    = "else"
	CondExpr    = "expr"
)

type Condition struct {
	Kind string // one of constants above
	Expr *Expr  // present if Kind==CondExpr
}

// Expr is a minimally structured expression for MVP.
// We keep tokens-as-text joined, optionally with a parsed identifier literal.
type Expr struct {
	Raw string
}

// IR Graph for visualization/runtime

type Graph struct {
	WorkflowName string
	WorkflowType string // workflow type (workflow, feature, application, etc.)
	Nodes        map[string]*Node
	Start        string
	Constants    map[string]interface{}
}

type Node struct {
	Name           string
	Action         *Action
	ParamNames     []string
	Edges          []Edge
	Start          bool
	Terminal       bool
	TerminalKind   string            // ok|fail
	Attr           map[string]string // for end nodes
	IfExpr         *Expr             // optional if condition expression
	ContinueOnFail bool              // continue on fail flag
	SkipTo         bool              // skip to flag
	// Parallel fork/join
	Parallel      bool
	ParallelCount int
	Wait          bool
	JoinTarget    string
}

type Edge struct {
	Condition Condition
	WhenExpr  *Expr // optional when condition expression
	To        string
	Args      []Expr
}
