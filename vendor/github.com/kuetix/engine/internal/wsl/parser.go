package wsl

import (
	"path/filepath"
	"strings"
)

// Recursive-descent parser producing CST

type parser struct {
	lx    *Lexer
	cur   Token
	ahead bool
}

func newParser(src string) *parser {
	p := &parser{lx: NewLexer(src)}
	p.next()
	return p
}

func (p *parser) next() {
	if p.ahead {
		p.ahead = false
		return
	}
	p.cur = p.lx.Next()
}

func (p *parser) expect(kind TokenKind) (Token, error) {
	t := p.cur
	if t.Kind != kind {
		return t, errf(t.Pos, "expected %s, got %s", kind, t.Kind)
	}
	p.cur = p.lx.Next()
	return t, nil
}

func (p *parser) accept(kind TokenKind) (Token, bool) {
	if p.cur.Kind == kind {
		t := p.cur
		p.cur = p.lx.Next()
		return t, true
	}
	return Token{}, false
}

// ParseCST parses source into CSTModule.
func ParseCST(src string, filename string) (*CSTModule, error) {
	p := newParser(src)

	// module <ident>
	var cst *CSTModule

	// Check if module keyword is present
	if p.cur.Kind == TokModule {
		// Parse explicit module declaration
		modTok := p.cur
		p.next()
		nameTok, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		cst = &CSTModule{Span: Span{Start: modTok.Pos}, NameTok: nameTok}
	} else {
		// Module keyword is optional - derive from filename if provided
		var moduleName string
		if filename != "" {
			// Extract module name from filename (remove path and extension)
			base := filepath.Base(filename)
			moduleName = strings.TrimSuffix(base, filepath.Ext(base))
		} else {
			// Default module name if no filename provided
			moduleName = "main"
		}

		// Create a synthetic token for the module name
		nameTok := Token{
			Kind:   TokIdent,
			Lexeme: moduleName,
			Pos:    p.cur.Pos,
		}
		cst = &CSTModule{Span: Span{Start: p.cur.Pos}, NameTok: nameTok}
	}

	// zero or more imports/context/constants/extends/workflows
	for p.cur.Kind != TokEOF {
		switch p.cur.Kind {
		case TokImport:
			imp, err := p.parseImport()
			if err != nil {
				return nil, err
			}
			cst.Imports = append(cst.Imports, *imp)
		case TokExtends:
			ext, err := p.parseExtends()
			if err != nil {
				return nil, err
			}
			cst.Extends = append(cst.Extends, *ext)
		case TokContext:
			if cst.Context != nil {
				return nil, errf(p.cur.Pos, "duplicate context block, here: ...%s...", p.lx.Peace(20))
			}
			ctx, err := p.parseContext()
			if err != nil {
				return nil, err
			}
			cst.Context = ctx
		case TokConst:
			if cst.Constants != nil {
				return nil, errf(p.cur.Pos, "duplicate const block, here: ...%s...", p.lx.Peace(20))
			}
			cb, err := p.parseConstBlock()
			if err != nil {
				return nil, err
			}
			cst.Constants = cb
		case TokWorkflow, TokIdent:
			// Handle workflow keyword or any identifier (e.g., feature, solution, etc.)
			wf, err := p.parseWorkflow()
			if err != nil {
				return nil, err
			}
			cst.Workflows = append(cst.Workflows, *wf)
		default:
			return nil, errf(p.cur.Pos, "unexpected token %s, here: ...%s...", p.cur.Kind, p.lx.Peace(20))
		}
	}
	cst.Span.End = p.cur.Pos
	return cst, nil
}

func (p *parser) parseExtends() (*CSTExtends, error) {
	extTok, err := p.expect(TokExtends)
	if err != nil {
		return nil, err
	}
	// next token must be string or ident
	if p.cur.Kind != TokString && p.cur.Kind != TokIdent {
		return nil, errf(p.cur.Pos, "expected string or identifier after 'extends', got %s, here: ...%s...", p.cur.Kind, p.lx.Peace(20))
	}
	val := p.cur
	p.next()
	ce := &CSTExtends{Span: Span{Start: extTok.Pos}, ExtTok: extTok, ValueTok: val}
	ce.Span.End = val.Pos
	return ce, nil
}

func (p *parser) parseConstBlock() (*CSTConstBlock, error) {
	constTok, err := p.expect(TokConst)
	if err != nil {
		return nil, err
	}
	lb, err := p.expect(TokLBrace)
	if err != nil {
		return nil, err
	}
	cb := &CSTConstBlock{Span: Span{Start: constTok.Pos}, ConstTok: constTok, LBrace: lb}
	// zero or more entries key: value separated by commas/newlines
	for p.cur.Kind != TokRBrace {
		if p.cur.Kind == TokEOF {
			return nil, errf(p.cur.Pos, "unexpected EOF in const block, here: ...%s...", p.lx.Peace(20))
		}
		// allow optional trailing commas and line breaks; require identifier key
		key, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		colon, err := p.expect(TokColon)
		if err != nil {
			return nil, err
		}
		// parse value (can be scalar, object, or array)
		val, err := p.parseConstValue()
		if err != nil {
			return nil, err
		}
		cb.Entries = append(cb.Entries, CSTConstEntry{Key: key, Colon: colon, Val: val})
		// optional comma
		if _, ok := p.accept(TokComma); ok {
			// continue to next entry
		}
		// allow trailing comma before '}' by continuing loop condition
		if p.cur.Kind == TokRBrace {
			break
		}
	}
	rb, err := p.expect(TokRBrace)
	if err != nil {
		return nil, err
	}
	cb.RBrace = rb
	cb.Span.End = rb.Pos
	return cb, nil
}

// parseConstValue parses a value in const block (scalar, object, or array)
func (p *parser) parseConstValue() (CSTValue, error) {
	switch p.cur.Kind {
	case TokLBrace:
		// object
		obj, err := p.parseConstObject()
		if err != nil {
			return CSTValue{}, err
		}
		return CSTValue{Kind: CSTValueObject, Object: obj}, nil
	case TokLBrack:
		// array
		arr, err := p.parseConstArray()
		if err != nil {
			return CSTValue{}, err
		}
		return CSTValue{Kind: CSTValueArray, Array: arr}, nil
	case TokString, TokNumber, TokIdent:
		// scalar value
		tok := p.cur
		p.next()
		return CSTValue{Kind: CSTValueScalar, Token: &tok}, nil
	default:
		return CSTValue{}, errf(p.cur.Pos, "expected const value, got %s, here: ...%s...", p.cur.Kind, p.lx.Peace(20))
	}
}

// parseConstObject parses an object literal { key: value, ... }
func (p *parser) parseConstObject() (*CSTObject, error) {
	lb, err := p.expect(TokLBrace)
	if err != nil {
		return nil, err
	}
	obj := &CSTObject{LBrace: lb}

	for p.cur.Kind != TokRBrace {
		if p.cur.Kind == TokEOF {
			return nil, errf(p.cur.Pos, "unexpected EOF in object, here: ...%s...", p.lx.Peace(20))
		}
		// key can be identifier or string
		if p.cur.Kind != TokIdent && p.cur.Kind != TokString {
			return nil, errf(p.cur.Pos, "expected object key (identifier or string), got %s, here: ...%s...", p.cur.Kind, p.lx.Peace(20))
		}
		key := p.cur
		p.next()

		colon, err := p.expect(TokColon)
		if err != nil {
			return nil, err
		}

		val, err := p.parseConstValue()
		if err != nil {
			return nil, err
		}

		obj.Entries = append(obj.Entries, CSTObjectEntry{Key: key, Colon: colon, Val: val})

		// optional comma
		if _, ok := p.accept(TokComma); ok {
			// continue to next entry
		}

		if p.cur.Kind == TokRBrace {
			break
		}
	}

	rb, err := p.expect(TokRBrace)
	if err != nil {
		return nil, err
	}
	obj.RBrace = rb
	return obj, nil
}

// parseConstArray parses an array literal [ value, ... ]
func (p *parser) parseConstArray() (*CSTArray, error) {
	lb, err := p.expect(TokLBrack)
	if err != nil {
		return nil, err
	}
	arr := &CSTArray{LBrack: lb}

	for p.cur.Kind != TokRBrack {
		if p.cur.Kind == TokEOF {
			return nil, errf(p.cur.Pos, "unexpected EOF in array, here: ...%s...", p.lx.Peace(20))
		}

		val, err := p.parseConstValue()
		if err != nil {
			return nil, err
		}

		arr.Values = append(arr.Values, val)

		// optional comma
		if _, ok := p.accept(TokComma); ok {
			// continue to next entry
		}

		if p.cur.Kind == TokRBrack {
			break
		}
	}

	rb, err := p.expect(TokRBrack)
	if err != nil {
		return nil, err
	}
	arr.RBrack = rb
	return arr, nil
}

func (p *parser) parseImport() (*CSTImport, error) {
	impTok := p.cur
	p.next()
	pathTok, err := p.expect(TokIdent)
	if err != nil {
		return nil, err
	}
	var asTok *Token
	var aliasTok *Token
	if t, ok := p.accept(TokAs); ok {
		asTok = &t
		a, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		aliasTok = &a
	}
	node := &CSTImport{Span: Span{Start: impTok.Pos, End: pathTok.Pos}, ImportTok: impTok, PathTok: pathTok, AsTok: asTok, AliasTok: aliasTok}
	return node, nil
}

func (p *parser) parseContext() (*CSTContext, error) {
	ctxTok := p.cur
	p.next()
	lbr, err := p.expect(TokLBrace)
	if err != nil {
		return nil, err
	}
	ctx := &CSTContext{Span: Span{Start: ctxTok.Pos}, LBrace: lbr}
	for p.cur.Kind != TokRBrace {
		if p.cur.Kind == TokEOF {
			return nil, errf(p.cur.Pos, "unexpected EOF in context, here: ...%s...", p.lx.Peace(20))
		}
		name, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		colon, err := p.expect(TokColon)
		if err != nil {
			return nil, err
		}
		typ, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		ctx.Fields = append(ctx.Fields, CSTField{Span: Span{Start: name.Pos, End: typ.Pos}, NameTok: name, ColonTok: colon, TypeTok: typ})
	}
	rbr, _ := p.expect(TokRBrace)
	ctx.RBrace = rbr
	ctx.Span.End = rbr.Pos
	return ctx, nil
}

func (p *parser) parseWorkflow() (*CSTWorkflow, error) {
	// Accept either TokWorkflow or any TokIdent as workflow type
	typeTok := p.cur
	if typeTok.Kind != TokWorkflow && typeTok.Kind != TokIdent {
		return nil, errf(p.cur.Pos, "expected workflow type keyword (workflow, feature, solution, etc.), got %s, here: ...%s...", typeTok.Kind, p.lx.Peace(20))
	}
	p.next()
	nameTok, err := p.expect(TokIdent)
	if err != nil {
		return nil, err
	}
	lbr, err := p.expect(TokLBrace)
	if err != nil {
		return nil, err
	}
	startTok, err := p.expect(TokStart)
	if err != nil {
		return nil, err
	}
	colon, err := p.expect(TokColon)
	if err != nil {
		return nil, err
	}
	startName, err := p.expect(TokIdent)
	if err != nil {
		return nil, err
	}
	wf := &CSTWorkflow{Span: Span{Start: typeTok.Pos}, TypeTok: typeTok, NameTok: nameTok, LBrace: lbr, StartTok: startTok, ColonTok: colon, StartName: startName}
	for p.cur.Kind != TokRBrace {
		if p.cur.Kind == TokEOF {
			return nil, errf(p.cur.Pos, "unexpected EOF in workflow, here: ...%s...", p.lx.Peace(20))
		}
		var st *CSTState
		var err error
		switch p.cur.Kind {
		case TokParallel:
			st, err = p.parseParallelState()
		case TokWait:
			st, err = p.parseWaitState()
		default:
			st, err = p.parseState()
		}
		if err != nil {
			return nil, err
		}
		wf.States = append(wf.States, *st)
	}
	rbr, _ := p.expect(TokRBrace)
	wf.RBrace = rbr
	wf.Span.End = rbr.Pos
	return wf, nil
}

func (p *parser) parseState() (*CSTState, error) {
	stTok, err := p.expect(TokState)
	if err != nil {
		return nil, err
	}
	return p.parseStateAfterKeyword(stTok)
}

// parseParallelState parses a parallel state:
//
//	parallel[count: 6] Name { action ...; on success -> Next }
//
// The bracketed attribute list is optional in the grammar but 'count' is
// required semantically (enforced in BuildAST). The state body is identical
// to a regular state body.
func (p *parser) parseParallelState() (*CSTState, error) {
	pTok, err := p.expect(TokParallel)
	if err != nil {
		return nil, err
	}
	attrs, err := p.parseOptionalBracketAttrs()
	if err != nil {
		return nil, err
	}
	st, err := p.parseStateAfterKeyword(pTok)
	if err != nil {
		return nil, err
	}
	st.Parallel = true
	st.ParallelAttrs = attrs
	return st, nil
}

// parseOptionalBracketAttrs parses an optional [key: value, ...] attribute
// list, e.g. the count in `parallel[count: 6]`. Shared between full WSL and
// SimplifiedWSL so both surfaces accept identical attribute syntax.
func (p *parser) parseOptionalBracketAttrs() ([]CSTConstEntry, error) {
	var attrs []CSTConstEntry
	if _, ok := p.accept(TokLBrack); !ok {
		return nil, nil
	}
	for p.cur.Kind != TokRBrack {
		if p.cur.Kind == TokEOF {
			return nil, errf(p.cur.Pos, "unexpected EOF in bracket attributes, here: ...%s...", p.lx.Peace(20))
		}
		key, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		colon, err := p.expect(TokColon)
		if err != nil {
			return nil, err
		}
		val, err := p.parseConstValue()
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, CSTConstEntry{Key: key, Colon: colon, Val: val})
		if _, ok := p.accept(TokComma); ok {
			continue
		}
		if p.cur.Kind == TokRBrack {
			break
		}
	}
	if _, err := p.expect(TokRBrack); err != nil {
		return nil, err
	}
	return attrs, nil
}

// parseWaitState parses a wait (join) state:
//
//	wait Name { join ParallelStateName; on success -> Next; on error -> Failed }
//
// A wait state has no action; its transitions fire after every branch of the
// joined parallel state has finished.
func (p *parser) parseWaitState() (*CSTState, error) {
	wTok, err := p.expect(TokWait)
	if err != nil {
		return nil, err
	}
	nameTok, err := p.expect(TokIdent)
	if err != nil {
		return nil, err
	}
	lbr, err := p.expect(TokLBrace)
	if err != nil {
		return nil, err
	}
	st := &CSTState{Span: Span{Start: wTok.Pos}, NameTok: nameTok, LBrace: lbr, Wait: true}
	if joinTok, ok := p.accept(TokJoin); ok {
		st.JoinTok = &joinTok
		target, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		st.JoinTarget = &target
	}
	for p.cur.Kind == TokOn {
		tr, err := p.parseTransition()
		if err != nil {
			return nil, err
		}
		st.Transitions = append(st.Transitions, *tr)
	}
	if p.cur.Kind == TokEnd {
		end, err := p.parseEnd()
		if err != nil {
			return nil, err
		}
		st.End = end
	}
	rbr, err := p.expect(TokRBrace)
	if err != nil {
		return nil, err
	}
	st.RBrace = rbr
	st.Span.End = rbr.Pos
	return st, nil
}

func (p *parser) parseStateAfterKeyword(stTok Token) (*CSTState, error) {
	nameTok, err := p.expect(TokIdent)
	if err != nil {
		return nil, err
	}
	st := &CSTState{Span: Span{Start: stTok.Pos}, NameTok: nameTok}
	// Optional parameter list: state Name(p1, p2)
	if t, ok := p.accept(TokLParen); ok {
		st.LParen = &t
		// zero or more idents separated by commas
		if p.cur.Kind != TokRParen {
			for {
				ptok, err := p.expect(TokIdent)
				if err != nil {
					return nil, err
				}
				st.Params = append(st.Params, ptok)
				if _, ok := p.accept(TokComma); ok {
					continue
				}
				break
			}
		}
		rp, err := p.expect(TokRParen)
		if err != nil {
			return nil, err
		}
		st.RParen = &rp
	}
	lbr, err := p.expect(TokLBrace)
	if err != nil {
		return nil, err
	}
	st.LBrace = lbr

	// Parse optional state attributes before action
	// Check for 'if <expression>'
	if p.cur.Kind == TokIf {
		ifTok := p.cur
		st.IfTok = &ifTok
		p.next()
		// Parse expression until we hit 'action', 'on', 'end', 'continue', 'skip', or '}'
		ifExpr, err := p.parseStateAttrExpr()
		if err != nil {
			return nil, err
		}
		st.IfExpr = ifExpr
	}
	// Check for 'continue on fail'
	if p.cur.Kind == TokContinue {
		p.next()
		if _, err := p.expect(TokOn); err != nil {
			return nil, errf(p.cur.Pos, "expected 'on' after 'continue', got %s, here: ...%s...", p.cur.Kind, p.lx.Peace(20))
		}
		if _, err := p.expect(TokFail); err != nil {
			return nil, errf(p.cur.Pos, "expected 'fail' after 'continue on', got %s, here: ...%s...", p.cur.Kind, p.lx.Peace(20))
		}
		st.ContinueOnFail = true
	}
	// Check for 'skip to'
	if p.cur.Kind == TokSkip {
		p.next()
		// Expect 'to' but it's not a token, so check for ident "to"
		if p.cur.Kind == TokIdent && p.cur.Lexeme == "to" {
			p.next()
			st.SkipTo = true
		} else {
			return nil, errf(p.cur.Pos, "expected 'to' after 'skip', got %s '%s', here: ...%s...", p.cur.Kind, p.cur.Lexeme, p.lx.Peace(20))
		}
	}

	// optional action
	if p.cur.Kind == TokAction {
		act, err := p.parseAction()
		if err != nil {
			return nil, err
		}
		st.Action = act
	}
	// zero or more transitions
	for p.cur.Kind == TokOn {
		tr, err := p.parseTransition()
		if err != nil {
			return nil, err
		}
		st.Transitions = append(st.Transitions, *tr)
	}
	// optional end
	if p.cur.Kind == TokEnd {
		end, err := p.parseEnd()
		if err != nil {
			return nil, err
		}
		st.End = end
	}
	rbr, err := p.expect(TokRBrace)
	if err != nil {
		return nil, err
	}
	st.RBrace = rbr
	st.Span.End = rbr.Pos
	return st, nil
}

// subWorkflowKinds lists the ident literals that act as sub-workflow type
// qualifiers in "action <type> <name>" statements (e.g., "action feature foo").
// "workflow" is already a dedicated keyword (TokWorkflow) and is handled
// separately; these are the common-ident equivalents.
var subWorkflowKinds = map[string]struct{}{
	"feature":  {},
	"solution": {},
}

func (p *parser) parseAction() (*CSTAction, error) {
	actTok := p.cur
	p.next()

	// Handle "action workflow|feature|solution <name>" sub-workflow invocation.
	// The sub-type qualifier is either the TokWorkflow keyword or one of the
	// known idents in subWorkflowKinds.  An optional name follows (which may be
	// a full path such as "ns/mod.name"); no parentheses are used.
	isSubWorkflow := p.cur.Kind == TokWorkflow ||
		(p.cur.Kind == TokIdent && isSubWorkflowIdent(p.cur.Lexeme))
	if isSubWorkflow {
		subTypeTok := p.cur
		subType := subTypeTok.Lexeme
		p.next()
		// Combine type and name into a space-separated lexeme so that downstream
		// code receives the exact "workflow <name>" string the engine expects.
		qnamePos := subTypeTok.Pos
		qnameLexeme := subType
		if p.cur.Kind == TokIdent {
			qnamePos = p.cur.Pos
			qnameLexeme = subType + " " + p.cur.Lexeme
			p.next()
		}
		syntheticQName := Token{Kind: TokIdent, Lexeme: qnameLexeme, Pos: qnamePos}
		a := &CSTAction{Span: Span{Start: actTok.Pos}, ActionTok: actTok, QNameTok: syntheticQName}
		if t, ok := p.accept(TokAs); ok {
			a.AsTok = &t
			alias, err := p.expect(TokIdent)
			if err != nil {
				return nil, err
			}
			a.AliasTok = &alias
			a.Span.End = alias.Pos
		} else {
			a.Span.End = syntheticQName.Pos
		}
		return a, nil
	}

	qname, err := p.expect(TokIdent)
	if err != nil {
		return nil, err
	}
	lp, err := p.expect(TokLParen)
	if err != nil {
		return nil, err
	}
	a := &CSTAction{Span: Span{Start: actTok.Pos}, ActionTok: actTok, QNameTok: qname, LParen: lp}
	// args list: zero or more expressions separated by comma until ')'
	if p.cur.Kind != TokRParen {
		for {
			ex, err := p.parseExprUntilCommaOrParen()
			if err != nil {
				return nil, err
			}
			a.Args = append(a.Args, *ex)
			if _, ok := p.accept(TokComma); ok {
				continue
			}
			break
		}
	}
	rp, err := p.expect(TokRParen)
	if err != nil {
		return nil, err
	}
	a.RParen = rp
	if t, ok := p.accept(TokAs); ok {
		a.AsTok = &t
		alias, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		a.AliasTok = &alias
	}
	a.Span.End = a.RParen.Pos
	if a.AliasTok != nil {
		a.Span.End = a.AliasTok.Pos
	}
	return a, nil
}

// isSubWorkflowIdent reports whether lex is a sub-workflow type ident.
func isSubWorkflowIdent(lex string) bool {
	_, ok := subWorkflowKinds[lex]
	return ok
}

func (p *parser) parseTransition() (*CSTTransition, error) {
	onTok := p.cur
	p.next()
	cond, err := p.parseCondition()
	if err != nil {
		return nil, err
	}
	arrow, err := p.expect(TokArrow)
	if err != nil {
		return nil, err
	}
	target, err := p.expect(TokIdent)
	if err != nil {
		return nil, err
	}
	tr := &CSTTransition{Span: Span{Start: onTok.Pos}, OnTok: onTok, Cond: *cond, ArrowTok: arrow, TargetTok: target}
	// Optional argument list after target
	if t, ok := p.accept(TokLParen); ok {
		tr.LParen = &t
		if p.cur.Kind != TokRParen {
			for {
				ex, err := p.parseExprUntilCommaOrParen()
				if err != nil {
					return nil, err
				}
				tr.Args = append(tr.Args, *ex)
				if _, ok := p.accept(TokComma); ok {
					continue
				}
				break
			}
		}
		rp, err := p.expect(TokRParen)
		if err != nil {
			return nil, err
		}
		tr.RParen = &rp
		tr.Span.End = rp.Pos
	} else {
		tr.Span.End = target.Pos
	}
	return tr, nil
}

func (p *parser) parseEnd() (*CSTEnd, error) {
	endTok := p.cur
	p.next()
	kindTok, err := p.expectOneOf(TokOk, TokFail)
	if err != nil {
		return nil, err
	}
	e := &CSTEnd{Span: Span{Start: endTok.Pos}, EndTok: endTok, KindTok: kindTok}
	// attrs: key=value pairs until end of block or newline; we parse greedily until '}' or next keyword that starts a new construct doesn't appear here; simplest: parse zero or more key=val while next is Ident
	for p.cur.Kind == TokIdent {
		key := p.cur
		p.next()
		eq, err := p.expect(TokEqual)
		if err != nil {
			return nil, err
		}
		// value can be string, ident, number
		switch p.cur.Kind {
		case TokString, TokIdent, TokNumber:
			val := p.cur
			p.next()
			e.Attrs = append(e.Attrs, CSTAttr{Key: key, Eq: eq, Val: val})
		default:
			return nil, errf(p.cur.Pos, "expected string|ident|number after '=', got %s, here: ...%s...", p.cur.Kind, p.lx.Peace(20))
		}
	}
	e.Span.End = e.KindTok.Pos
	if n := len(e.Attrs); n > 0 {
		e.Span.End = e.Attrs[n-1].Val.Pos
	}
	return e, nil
}

func (p *parser) expectOneOf(k1, k2 TokenKind) (Token, error) {
	if p.cur.Kind == k1 || p.cur.Kind == k2 {
		t := p.cur
		p.next()
		return t, nil
	}
	return p.cur, errf(p.cur.Pos, "expected %s or %s, got %s, here: ...%s...", k1, k2, p.cur.Kind, p.lx.Peace(20))
}

func (p *parser) parseCondition() (*CSTCondition, error) {
	c := &CSTCondition{}
	c.Span.Start = p.cur.Pos
	switch p.cur.Kind {
	case TokSuccess, TokError, TokFail, TokElse:
		c.Kind = p.cur.Kind
		c.Span.End = p.cur.Pos
		p.next()
		// Check for optional 'when' expression
		if p.cur.Kind == TokWhen {
			whenTok := p.cur
			c.WhenTok = &whenTok
			p.next()
			// Parse expression until arrow
			whenExpr, err := p.parseExprUntilArrow()
			if err != nil {
				return nil, err
			}
			c.WhenExpr = whenExpr
			c.Span.End = whenExpr.Span.End
		}
		return c, nil
	default:
		ex, err := p.parseExprUntilArrow()
		if err != nil {
			return nil, err
		}
		c.Kind = TokIdent
		c.Expr = ex
		c.Span.End = ex.Span.End
		return c, nil
	}
}

// tokenLexeme returns the raw source representation of a token, including
// surrounding quotes for string literals so that expressions can be accurately
// reconstructed from their constituent tokens.
func tokenLexeme(tok Token) string {
	if tok.Kind == TokString {
		return "\"" + tok.Lexeme + "\""
	}
	return tok.Lexeme
}

func (p *parser) parseExprUntilArrow() (*CSTExpr, error) {
	// collect raw text until we see '->'
	start := p.cur.Pos
	raw := ""
	for p.cur.Kind != TokArrow && p.cur.Kind != TokEOF && p.cur.Kind != TokRBrace {
		// include whitespace between tokens by inserting single space
		if len(raw) > 0 {
			raw += " "
		}
		raw += tokenLexeme(p.cur)
		p.next()
	}
	if raw == "" {
		return nil, errf(p.cur.Pos, "empty expression, expected condition before '->', here: ...%s...", p.lx.Peace(20))
	}
	return &CSTExpr{Raw: raw, Span: Span{Start: start, End: p.cur.Pos}}, nil
}

func (p *parser) parseExprUntilCommaOrParen() (*CSTExpr, error) {
	start := p.cur.Pos
	raw := ""
	parenDepth := 0
	braceDepth := 0
	brackDepth := 0
	for p.cur.Kind != TokEOF {
		if p.cur.Kind == TokRParen && parenDepth == 0 && braceDepth == 0 && brackDepth == 0 {
			break
		}
		if p.cur.Kind == TokComma && parenDepth == 0 && braceDepth == 0 && brackDepth == 0 {
			break
		}
		switch p.cur.Kind {
		case TokLParen:
			parenDepth++
		case TokRParen:
			if parenDepth > 0 {
				parenDepth--
			}
		case TokLBrace:
			braceDepth++
		case TokRBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case TokLBrack:
			brackDepth++
		case TokRBrack:
			if brackDepth > 0 {
				brackDepth--
			}
		default:
		}
		if len(raw) > 0 {
			raw += " "
		}
		raw += tokenLexeme(p.cur)
		p.next()
	}
	return &CSTExpr{Raw: raw, Span: Span{Start: start, End: p.cur.Pos}}, nil
}

func (p *parser) parseStateAttrExpr() (*CSTExpr, error) {
	// Parse expression until we hit a state-level keyword or closing brace
	// Stop tokens: action, on, end, continue, skip, }
	start := p.cur.Pos
	raw := ""
	for p.cur.Kind != TokEOF {
		if p.cur.Kind == TokAction || p.cur.Kind == TokOn || p.cur.Kind == TokEnd ||
			p.cur.Kind == TokContinue || p.cur.Kind == TokSkip || p.cur.Kind == TokRBrace {
			break
		}
		if len(raw) > 0 {
			raw += " "
		}
		raw += tokenLexeme(p.cur)
		p.next()
	}
	if raw == "" {
		return nil, errf(p.cur.Pos, "empty expression after 'if', expected condition, here: ...%s...", p.lx.Peace(20))
	}
	return &CSTExpr{Raw: raw, Span: Span{Start: start, End: p.cur.Pos}}, nil
}
