package wsl

// TokenKind represents lexical token types.
type TokenKind int

const (
	// Special
	TokEOF TokenKind = iota
	TokIllegal

	// Identifiers and literals
	TokIdent
	TokNumber
	TokString

	// Keywords
	TokModule
	TokImport
	TokAs
	TokContext
	TokWorkflow
	TokStart
	TokState
	TokAction
	TokOn
	TokEnd
	TokFail
	TokOk
	TokConst
	TokExtends
	TokDef
	TokParallel
	TokWait
	TokJoin

	// Condition keywords
	TokSuccess
	TokError
	TokElse
	TokWhen
	TokIf
	TokContinue
	TokSkip

	// Punctuators/operators
	TokLBrace    // {
	TokRBrace    // }
	TokLParen    // (
	TokRParen    // )
	TokLBrack    // [
	TokRBrack    // ]
	TokColon     // :
	TokComma     // ,
	TokArrow     // ->
	TokLeftArrow // <-
	TokDot       // .
	TokEqual     // =
	TokPipe      // |
	TokGt        // >
	TokLt        // <
	TokBang      // !
)

// Position tracks a point in source.
type Position struct {
	Offset int // byte offset from start
	Line   int // 1-based
	Col    int // 1-based, in runes
}

// Span marks a source range.
type Span struct {
	Start Position
	End   Position
}

// Token is a lexical token with optional literal text.
type Token struct {
	Kind   TokenKind
	Lexeme string // raw text (for identifiers/strings/numbers or expr chunks)
	Pos    Position
}

func (k TokenKind) String() string {
	switch k {
	case TokEOF:
		return "EOF"
	case TokIllegal:
		return "Illegal"
	case TokIdent:
		return "Ident"
	case TokNumber:
		return "Number"
	case TokString:
		return "String"
	case TokModule:
		return "module"
	case TokImport:
		return "import"
	case TokAs:
		return "as"
	case TokContext:
		return "context"
	case TokWorkflow:
		return "workflow"
	case TokStart:
		return "start"
	case TokState:
		return "state"
	case TokAction:
		return "action"
	case TokOn:
		return "on"
	case TokEnd:
		return "end"
	case TokFail:
		return "fail"
	case TokOk:
		return "ok"
	case TokSuccess:
		return "success"
	case TokError:
		return "error"
	case TokElse:
		return "else"
	case TokWhen:
		return "when"
	case TokIf:
		return "if"
	case TokContinue:
		return "continue"
	case TokSkip:
		return "skip"
	case TokConst:
		return "{"
	case TokRBrace:
		return "}"
	case TokLParen:
		return "("
	case TokRParen:
		return ")"
	case TokLBrack:
		return "["
	case TokRBrack:
		return "]"
	case TokColon:
		return ":"
	case TokComma:
		return ","
	case TokArrow:
		return "->"
	case TokLeftArrow:
		return "<-"
	case TokDot:
		return "."
	case TokEqual:
		return "="
	case TokExtends:
		return "extends"
	case TokDef:
		return "def"
	case TokParallel:
		return "parallel"
	case TokWait:
		return "wait"
	case TokJoin:
		return "join"
	case TokGt:
		return ">"
	case TokLt:
		return "<"
	case TokBang:
		return "!"
	default:
		return "?"
	}
}
