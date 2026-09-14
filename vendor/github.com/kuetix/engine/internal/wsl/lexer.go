package wsl

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// Lexer implements a simple hand-written lexer for WSL.
type Lexer struct {
	src    string
	offset int
	line   int
	col    int
}

func NewLexer(src string) *Lexer {
	return &Lexer{src: src, line: 1, col: 1}
}

func (lx *Lexer) pos() Position { return Position{Offset: lx.offset, Line: lx.line, Col: lx.col} }

func (lx *Lexer) nextRune() (r rune, size int) {
	if lx.offset >= len(lx.src) {
		return 0, 0
	}
	r, size = utf8.DecodeRuneInString(lx.src[lx.offset:])
	return r, size
}

func (lx *Lexer) advance(size int) {
	if size == 0 {
		return
	}
	r := lx.src[lx.offset : lx.offset+size]
	// count newlines and cols
	for len(r) > 0 {
		c, s := utf8.DecodeRuneInString(r)
		if c == '\n' {
			lx.line++
			lx.col = 1
		} else {
			lx.col++
		}
		r = r[s:]
	}
	lx.offset += size
}

func (lx *Lexer) peekString(n int) string {
	if lx.offset+n > len(lx.src) {
		return lx.src[lx.offset:]
	}
	return lx.src[lx.offset : lx.offset+n]
}

func (lx *Lexer) skipSpacesAndComments() {
	for {
		// skip whitespace
		for {
			r, s := lx.nextRune()
			if s == 0 {
				return
			}
			if unicode.IsSpace(r) {
				lx.advance(s)
				continue
			}
			break
		}
		// line comments: // ... or # ...
		if lx.peekString(2) == "//" {
			// consume till end of line
			for {
				r, s := lx.nextRune()
				if s == 0 {
					return
				}
				lx.advance(s)
				if r == '\n' {
					break
				}
			}
			continue
		}
		if lx.peekString(1) == "#" {
			for {
				r, s := lx.nextRune()
				if s == 0 {
					return
				}
				lx.advance(s)
				if r == '\n' {
					break
				}
			}
			continue
		}
		return
	}
}

func isDollarVariableSymbol(r rune) bool {
	switch r {
	case '!', '@', '#', '%', '^', '&', '*', '_', '-', '+', '=', '~', '?':
		return true
	default:
		return false
	}
}

func isIdentStart(r rune) bool { return r == '_' || r == '$' || r == '-' || unicode.IsLetter(r) }
func isIdentPart(r rune) bool  { return isIdentStart(r) || unicode.IsDigit(r) }

func isQNameSeparator(r rune) bool {
	return r == '.' || r == '/'
}

func isValidIdentifierRune(r rune, startsWithDollar bool) bool {
	// Dot/slash are always allowed to support qualified names (e.g. ns/mod.name).
	if isIdentPart(r) || isQNameSeparator(r) {
		return true
	}
	// Extra symbols are only valid for $-prefixed variables.
	return startsWithDollar && isDollarVariableSymbol(r)
}

// Next returns the next token.
func (lx *Lexer) Next() Token {
	lx.skipSpacesAndComments()
	pos := lx.pos()
	r, s := lx.nextRune()
	if s == 0 {
		return Token{Kind: TokEOF, Pos: pos}
	}

	// Arrow '->'
	if lx.peekString(2) == "->" {
		lx.advance(2)
		return Token{Kind: TokArrow, Lexeme: "->", Pos: pos}
	}

	// Left arrow '<-'
	if lx.peekString(2) == "<-" {
		lx.advance(2)
		return Token{Kind: TokLeftArrow, Lexeme: "<-", Pos: pos}
	}

	switch r {
	case '{':
		lx.advance(s)
		return Token{Kind: TokLBrace, Lexeme: "{", Pos: pos}
	case '}':
		lx.advance(s)
		return Token{Kind: TokRBrace, Lexeme: "}", Pos: pos}
	case '(':
		lx.advance(s)
		return Token{Kind: TokLParen, Lexeme: "(", Pos: pos}
	case ')':
		lx.advance(s)
		return Token{Kind: TokRParen, Lexeme: ")", Pos: pos}
	case '[':
		lx.advance(s)
		return Token{Kind: TokLBrack, Lexeme: "[", Pos: pos}
	case ']':
		lx.advance(s)
		return Token{Kind: TokRBrack, Lexeme: "]", Pos: pos}
	case ':':
		lx.advance(s)
		return Token{Kind: TokColon, Lexeme: ":", Pos: pos}
	case ',':
		lx.advance(s)
		return Token{Kind: TokComma, Lexeme: ",", Pos: pos}
	case '.':
		lx.advance(s)
		return Token{Kind: TokDot, Lexeme: ".", Pos: pos}
	case '=':
		lx.advance(s)
		return Token{Kind: TokEqual, Lexeme: "=", Pos: pos}
	case '|':
		lx.advance(s)
		return Token{Kind: TokPipe, Lexeme: "|", Pos: pos}
	case '>':
		lx.advance(s)
		return Token{Kind: TokGt, Lexeme: ">", Pos: pos}
	case '<':
		lx.advance(s)
		return Token{Kind: TokLt, Lexeme: "<", Pos: pos}
	case '!':
		lx.advance(s)
		return Token{Kind: TokBang, Lexeme: "!", Pos: pos}
	case '"':
		// string literal
		lx.advance(s)
		start := lx.offset
		for {
			r2, s2 := lx.nextRune()
			if s2 == 0 {
				return Token{Kind: TokIllegal, Lexeme: "unterminated string", Pos: pos}
			}
			if r2 == '\\' { // escape, skip next
				lx.advance(s2)
				_, s3 := lx.nextRune()
				if s3 == 0 {
					return Token{Kind: TokIllegal, Lexeme: "unterminated escape", Pos: pos}
				}
				lx.advance(s3)
				continue
			}
			if r2 == '"' {
				lit := lx.src[start:lx.offset]
				lx.advance(s2)
				return Token{Kind: TokString, Lexeme: lit, Pos: pos}
			}
			lx.advance(s2)
		}
	}

	// number literal (simple: digits with optional dot)
	if unicode.IsDigit(r) {
		start := lx.offset
		lx.advance(s)
		for {
			r2, s2 := lx.nextRune()
			if s2 == 0 {
				break
			}
			if unicode.IsDigit(r2) || r2 == '_' || r2 == '.' {
				lx.advance(s2)
				continue
			}
			break
		}
		return Token{Kind: TokNumber, Lexeme: lx.src[start:lx.offset], Pos: pos}
	}

	// identifier or keyword
	if isIdentStart(r) {
		start := lx.offset
		lx.advance(s)
		for {
			r2, s2 := lx.nextRune()
			if s2 == 0 {
				break
			}
			startsWithDollar := lx.src[start] == '$'
			if isValidIdentifierRune(r2, startsWithDollar) { // allow dot/slash in qname and extra symbols in $variables
				// we include '.' and '/' here to allow qname as single token when no spaces, but parser can also accept separated dots
				lx.advance(s2)
				continue
			}
			break
		}
		lit := lx.src[start:lx.offset]
		kind := keywordKind(lit)
		if kind != TokIdent {
			return Token{Kind: kind, Lexeme: lit, Pos: pos}
		}
		return Token{Kind: TokIdent, Lexeme: lit, Pos: pos}
	}

	// unknown
	lx.advance(s)
	return Token{Kind: TokIllegal, Lexeme: fmt.Sprintf("unexpected %q", r), Pos: pos}
}

func keywordKind(s string) TokenKind {
	switch s {
	case "module":
		return TokModule
	case "import":
		return TokImport
	case "as":
		return TokAs
	case "context":
		return TokContext
	case "workflow":
		return TokWorkflow
	case "start":
		return TokStart
	case "state":
		return TokState
	case "action":
		return TokAction
	case "on":
		return TokOn
	case "end":
		return TokEnd
	case "fail":
		return TokFail
	case "ok":
		return TokOk
	case "success":
		return TokSuccess
	case "error":
		return TokError
	case "else":
		return TokElse
	case "when":
		return TokWhen
	case "if":
		return TokIf
	case "continue":
		return TokContinue
	case "skip":
		return TokSkip
	case "const":
		return TokConst
	case "extends":
		return TokExtends
	case "def":
		return TokDef
	case "parallel":
		return TokParallel
	case "wait":
		return TokWait
	case "join":
		return TokJoin
	default:
		return TokIdent
	}
}

func (lx *Lexer) Error() string {
	return fmt.Sprintf("lexer error at %s:%d:%d", lx.src, lx.line, lx.col)
}

func (lx *Lexer) Peace(length ...int) string {
	var l int
	if len(length) > 0 {
		l = length[0]
	} else {
		l = 20
	}
	start := lx.offset - l
	if start < 0 {
		start = 0
	}
	stop := lx.offset + l
	if stop > len(lx.src) {
		am := stop - len(lx.src)
		start = start - am
		if start < 0 {
			start = 0
		}
		stop = len(lx.src)
	}
	peaceOfPosCode := lx.src[start:stop]
	atPos := lx.offset - start
	peaceOfPosCode = peaceOfPosCode[:atPos] + "|" + peaceOfPosCode[atPos:]
	return peaceOfPosCode
}
