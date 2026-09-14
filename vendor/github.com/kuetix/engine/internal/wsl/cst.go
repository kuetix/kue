package wsl

// CST (Concrete Syntax Tree) nodes retain token/position info close to source.

type CSTModule struct {
	Span      Span
	NameTok   Token
	Imports   []CSTImport
	Extends   []CSTExtends   // optional, multiple
	Context   *CSTContext    // optional
	Constants *CSTConstBlock // optional
	Workflows []CSTWorkflow
}

type CSTImport struct {
	Span      Span
	ImportTok Token
	PathTok   Token // ident or qname as TokIdent
	AsTok     *Token
	AliasTok  *Token
}

// CSTExtends represents an extends directive at module level: extends "name"
// ValueTok can be TokString or TokIdent
type CSTExtends struct {
	Span     Span
	ExtTok   Token
	ValueTok Token
}

type CSTContext struct {
	Span   Span
	LBrace Token
	Fields []CSTField
	RBrace Token
}

type CSTField struct {
	Span     Span
	NameTok  Token
	ColonTok Token
	TypeTok  Token
}

type CSTWorkflow struct {
	Span      Span
	TypeTok   Token // workflow type keyword (workflow, feature, solution, etc.)
	NameTok   Token
	LBrace    Token
	StartTok  Token // 'start'
	ColonTok  Token
	StartName Token // ident
	States    []CSTState
	RBrace    Token
}

type CSTState struct {
	Span    Span
	NameTok Token
	// Optional parameter list: state Name(p1, p2)
	LParen *Token
	Params []Token
	RParen *Token
	LBrace Token
	// Optional state attributes
	IfTok          *Token   // 'if' keyword
	IfExpr         *CSTExpr // if condition expression
	ContinueOnFail bool     // 'continue on fail' flag
	SkipTo         bool     // 'skip to' flag
	// Parallel fork state: parallel[count: N] Name { ... }
	Parallel      bool
	ParallelAttrs []CSTConstEntry // attributes from the [ ... ] list (count, ...)
	// Wait (join) state: wait Name { join Target ... }
	Wait        bool
	JoinTok     *Token
	JoinTarget  *Token
	Action      *CSTAction
	Transitions []CSTTransition
	End         *CSTEnd
	RBrace      Token
}

type CSTAction struct {
	Span      Span
	ActionTok Token
	QNameTok  Token // module.fn or qname
	LParen    Token
	Args      []CSTExpr
	RParen    Token
	AsTok     *Token
	AliasTok  *Token
}

type CSTTransition struct {
	Span      Span
	OnTok     Token
	Cond      CSTCondition
	ArrowTok  Token
	TargetTok Token
	// Optional argument list after target: Target(arg1, arg2)
	LParen *Token
	Args   []CSTExpr
	RParen *Token
}

type CSTEnd struct {
	Span    Span
	EndTok  Token
	KindTok Token     // fail|ok
	Attrs   []CSTAttr // key=value pairs (values are strings or idents)
}

type CSTAttr struct {
	Key Token
	Eq  Token
	Val Token // TokString or TokIdent or TokNumber
}

// Conditions can be keywords or generic expression.
// Conditions can be keywords or generic expression.
type CSTCondition struct {
	Kind     TokenKind // TokSuccess|TokError|TokFail|TokElse or TokIdent as generic
	Expr     *CSTExpr  // present when not a keyword
	WhenTok  *Token    // optional 'when' keyword
	WhenExpr *CSTExpr  // optional when expression
	Span     Span
}

type CSTExpr struct {
	// For MVP we keep raw string of expression text and span
	Raw  string
	Span Span
}

type CSTConstBlock struct {
	Span     Span
	ConstTok Token
	LBrace   Token
	Entries  []CSTConstEntry
	RBrace   Token
}

type CSTConstEntry struct {
	Key   Token // identifier
	Colon Token
	Val   CSTValue // value can be scalar, object, or array
}

// CSTValue represents a value in const block - can be scalar, object, or array
type CSTValue struct {
	Kind   CSTValueKind
	Token  *Token     // for scalar values (string, number, ident, bool)
	Object *CSTObject // for object values
	Array  *CSTArray  // for array values
}

type CSTValueKind int

const (
	CSTValueScalar CSTValueKind = iota
	CSTValueObject
	CSTValueArray
)

// CSTObject represents an object literal { key: value, ... }
type CSTObject struct {
	LBrace  Token
	Entries []CSTObjectEntry
	RBrace  Token
}

type CSTObjectEntry struct {
	Key   Token // identifier or string
	Colon Token
	Val   CSTValue
}

// CSTArray represents an array literal [ value, ... ]
type CSTArray struct {
	LBrack Token
	Values []CSTValue
	RBrack Token
}
