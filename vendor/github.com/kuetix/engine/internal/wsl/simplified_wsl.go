package wsl

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// SimplifiedWSL parser - a streamlined syntax without workflow wrappers
//
// Syntax:
// module name
//
// const {
//   ...
// }
//
// errors.OnAnyError() as err -> .
// speak.Say(on: ...) <- err -> common.Response(...) <- onResponseError -> .

// ParseSimplifiedWSL parses simplified WSL syntax into a standard Module AST
func ParseSimplifiedWSL(src string) (*Module, error) {
	return ParseSimplifiedWSLWithFilename(src, "")
}

// ParseSimplifiedWSLWithFilename parses simplified WSL syntax into a standard Module AST
// If filename is provided and module keyword is missing, derives module name from filename
func ParseSimplifiedWSLWithFilename(src string, filename string) (*Module, error) {
	cst, err := parseSimplifiedCSTWithFilename(src, filename)
	if err != nil {
		return nil, err
	}
	return buildSimplifiedAST(cst)
}

// SimplifiedCSTModule represents the CST for simplified WSL
type SimplifiedCSTModule struct {
	Span         Span
	NameTok      Token
	WorkflowType *Token // optional workflow type (feature, solution, etc.)
	WorkflowName *Token // optional workflow name
	Imports      []CSTImport
	Constants    *CSTConstBlock
	Actions      []SimplifiedCSTAction
}

// SimplifiedCSTAction represents an action with its flow connections
type SimplifiedCSTAction struct {
	Span     Span
	DefTok   *Token // optional 'def' keyword
	QNameTok Token  // module.fn or qname
	LParen   Token
	Args     []CSTExpr
	RParen   Token
	// Optional [count: N] fork attributes, e.g. action()[count: 6]. When
	// present, this action call lowers to a parallel-fork state joined by a
	// synthetic wait state immediately after it (see buildParallelStatePair).
	ParallelAttrs      []CSTConstEntry
	AsTok              *Token
	AliasTok           *Token
	ErrorBindingRef    *Token               // optional error handler reference (via <-)
	ErrorBindingAction *SimplifiedCSTAction // optional inline error handler action (via <-)
	NextFlow           *SimplifiedCSTFlow   // optional next flow (via ->)
}

// SimplifiedCSTFlow represents a flow connection
type SimplifiedCSTFlow struct {
	Span     Span
	ArrowTok Token // ->
	Target   *SimplifiedCSTAction
	Ref      *Token // reference to a def alias (identifier without '('), used for success binding
	Terminal bool   // true if target is '.'
}

// parseSimplifiedCSTWithFilename parses simplified WSL source into CST
// If filename is provided and module keyword is missing, derives module name from filename
func parseSimplifiedCSTWithFilename(src string, filename string) (*SimplifiedCSTModule, error) {
	p := newParser(src)

	var cst *SimplifiedCSTModule

	// Check if module keyword is present
	if p.cur.Kind == TokModule {
		// Parse explicit module declaration
		modTok := p.cur
		p.next()
		nameTok, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		cst = &SimplifiedCSTModule{Span: Span{Start: modTok.Pos}, NameTok: nameTok}
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
		cst = &SimplifiedCSTModule{Span: Span{Start: p.cur.Pos}, NameTok: nameTok}
	}

	// Parse imports, constants, workflow type, and actions
	for p.cur.Kind != TokEOF {
		switch p.cur.Kind {
		case TokImport:
			imp, err := p.parseImport()
			if err != nil {
				return nil, err
			}
			cst.Imports = append(cst.Imports, *imp)
		case TokConst:
			if cst.Constants != nil {
				return nil, errf(p.cur.Pos, "duplicate const block, here: ...%s...", p.lx.Peace(20))
			}
			cb, err := p.parseConstBlock()
			if err != nil {
				return nil, err
			}
			cst.Constants = cb
		case TokWorkflow:
			// Parse workflow type keyword followed by optional name
			if cst.WorkflowType != nil {
				return nil, errf(p.cur.Pos, "duplicate workflow type declaration, here: ...%s...", p.lx.Peace(20))
			}
			typeTok := p.cur
			cst.WorkflowType = &typeTok
			p.next()
			// Check for optional workflow name (plain identifier without dots/slashes)
			if p.cur.Kind == TokIdent && !strings.ContainsAny(p.cur.Lexeme, "./") {
				nameTok := p.cur
				cst.WorkflowName = &nameTok
				p.next()
			}
		case TokDef:
			// Parse action flow with 'def' keyword
			action, err := p.parseSimplifiedAction()
			if err != nil {
				return nil, err
			}
			cst.Actions = append(cst.Actions, *action)
		case TokIdent:
			// Could be a workflow type keyword or an action
			// Check if this looks like a workflow type declaration
			// (identifier at module level before any actions, optionally followed by another identifier)
			if cst.WorkflowType == nil && len(cst.Actions) == 0 {
				// If the identifier contains a dot or slash, it's an action qualified name
				if strings.ContainsAny(p.cur.Lexeme, "./") {
					// This is an action
					action, err := p.parseSimplifiedAction()
					if err != nil {
						return nil, err
					}
					cst.Actions = append(cst.Actions, *action)
				} else {
					// Plain identifier - peek ahead to see if it's followed by an identifier (without dot/slash) or something else
					saved := p.cur
					p.next()
					if p.cur.Kind == TokIdent && !strings.ContainsAny(p.cur.Lexeme, "./") {
						// This is a workflow type followed by name
						cst.WorkflowType = &saved
						nameTok := p.cur
						cst.WorkflowName = &nameTok
						p.next()
					} else if p.cur.Kind == TokLParen {
						// This is an action (e.g., "action(")
						p.cur = saved
						action, err := p.parseSimplifiedAction()
						if err != nil {
							return nil, err
						}
						cst.Actions = append(cst.Actions, *action)
					} else {
						// This is a workflow type without name
						cst.WorkflowType = &saved
						// p.cur is already at the next token
					}
				}
			} else {
				// Parse action flow
				action, err := p.parseSimplifiedAction()
				if err != nil {
					return nil, err
				}
				cst.Actions = append(cst.Actions, *action)
			}
		default:
			return nil, errf(p.cur.Pos, "unexpected token %s in simplified WSL, here: ...%s...", p.cur.Kind, p.lx.Peace(20))
		}
	}

	cst.Span.End = p.cur.Pos
	return cst, nil
}

// parseSimplifiedAction parses an action with optional error bindings and flows
// Format: [def] qname(args) [as alias] [<- errorHandler|errorAction()] [-> nextAction|defRef|.]
func (p *parser) parseSimplifiedAction() (*SimplifiedCSTAction, error) {
	// Check for optional 'def' keyword
	var defTok *Token
	if p.cur.Kind == TokDef {
		tok := p.cur
		defTok = &tok
		p.next()
	}

	// Parse qualified name (e.g., speak.Say, errors.OnAnyError)
	if p.cur.Kind != TokIdent {
		return nil, errf(p.cur.Pos, "expected action name, here: ...%s...", p.lx.Peace(20))
	}

	qnameTok := p.cur
	p.next()

	action, err := p.parseSimplifiedActionBody(qnameTok)
	if err != nil {
		return nil, err
	}
	action.DefTok = defTok
	return action, nil
}

// parseSimplifiedActionBody parses the body of an action (args, alias, error binding, next flow)
// starting from after the qualified name identifier has already been consumed.
func (p *parser) parseSimplifiedActionBody(qnameTok Token) (*SimplifiedCSTAction, error) {
	// Expect (
	lparen, err := p.expect(TokLParen)
	if err != nil {
		return nil, err
	}

	// Parse arguments
	var args []CSTExpr
	if p.cur.Kind != TokRParen {
		for {
			arg, err := p.parseExprUntilCommaOrParen()
			if err != nil {
				return nil, err
			}
			args = append(args, *arg)

			if p.cur.Kind == TokRParen {
				break
			}
			if _, ok := p.accept(TokComma); !ok {
				return nil, errf(p.cur.Pos, "expected ',' or ')' in action arguments, here: ...%s...", p.lx.Peace(20))
			}
		}
	}

	rparen, err := p.expect(TokRParen)
	if err != nil {
		return nil, err
	}

	// Optional [count: N] fork attributes, e.g. commands.Register()[count: 6].
	// This is SWSL's sugar for a parallel[count: N] state: the action forks
	// into N concurrent branches, immediately joined before the chain
	// continues (SWSL has no syntax for named states between fork and join,
	// so unlike full WSL the join is always implicit and immediate here).
	parallelAttrs, err := p.parseOptionalBracketAttrs()
	if err != nil {
		return nil, err
	}

	action := &SimplifiedCSTAction{
		Span:          Span{Start: qnameTok.Pos},
		QNameTok:      qnameTok,
		LParen:        lparen,
		Args:          args,
		RParen:        rparen,
		ParallelAttrs: parallelAttrs,
	}

	// Check for 'as' alias
	if p.cur.Kind == TokAs {
		asTok := p.cur
		p.next()
		aliasTok, err := p.expect(TokIdent)
		if err != nil {
			return nil, err
		}
		action.AsTok = &asTok
		action.AliasTok = &aliasTok
	}

	// Check for '<-' error binding (reference or inline action)
	if p.cur.Kind == TokLeftArrow {
		p.next()
		// Check if it's an identifier reference or an inline action
		if p.cur.Kind == TokIdent {
			// Save current identifier
			identTok := p.cur
			p.next()

			if p.cur.Kind == TokLParen {
				// It's an inline action - we already have the identifier and paren
				// Manually construct the action parsing
				lparen := p.cur
				p.next()

				// Parse arguments
				var args []CSTExpr
				if p.cur.Kind != TokRParen {
					for {
						arg, err := p.parseExprUntilCommaOrParen()
						if err != nil {
							return nil, err
						}
						args = append(args, *arg)

						if p.cur.Kind == TokRParen {
							break
						}
						if _, ok := p.accept(TokComma); !ok {
							return nil, errf(p.cur.Pos, "expected ',' or ')' in action arguments, here: ...%s...", p.lx.Peace(20))
						}
					}
				}

				rparen, err := p.expect(TokRParen)
				if err != nil {
					return nil, err
				}

				errorAction := &SimplifiedCSTAction{
					Span:     Span{Start: identTok.Pos},
					QNameTok: identTok,
					LParen:   lparen,
					Args:     args,
					RParen:   rparen,
				}

				// Check for optional 'as' alias
				if p.cur.Kind == TokAs {
					asTok := p.cur
					p.next()
					aliasTok, err := p.expect(TokIdent)
					if err != nil {
						return nil, err
					}
					errorAction.AsTok = &asTok
					errorAction.AliasTok = &aliasTok
				}

				errorAction.Span.End = p.cur.Pos
				action.ErrorBindingAction = errorAction
			} else {
				// It's a reference to a previously defined alias
				action.ErrorBindingRef = &identTok
			}
		} else {
			return nil, errf(p.cur.Pos, "expected identifier or action after '<-', here: ...%s...", p.lx.Peace(20))
		}
	}

	// Check for '->' flow
	if p.cur.Kind == TokArrow {
		arrowTok := p.cur
		p.next()

		flow := &SimplifiedCSTFlow{
			Span:     Span{Start: arrowTok.Pos},
			ArrowTok: arrowTok,
		}

		// Check if target is terminal '.'
		if p.cur.Kind == TokDot {
			flow.Terminal = true
			p.next()
		} else if p.cur.Kind == TokIdent {
			// Peek at the token after the identifier to determine whether this is:
			//   - a def alias reference: "-> identifier" (no '(' follows)
			//   - an inline action:      "-> identifier(args...)" ('(' follows)
			identTok := p.cur
			p.next()
			if p.cur.Kind == TokLParen {
				// It's an inline action: parse its body using the already-consumed identifier
				nextAction, err := p.parseSimplifiedActionBody(identTok)
				if err != nil {
					return nil, err
				}
				flow.Target = nextAction
			} else {
				// It's a reference to a def alias (no '(' follows)
				flow.Ref = &identTok
			}
		}

		action.NextFlow = flow
	}

	action.Span.End = p.cur.Pos
	return action, nil
}

// buildSimplifiedAST converts SimplifiedCST to standard AST Module
func buildSimplifiedAST(cst *SimplifiedCSTModule) (*Module, error) {
	mod := &Module{
		Name: cst.NameTok.Lexeme,
	}

	// Convert imports
	for _, imp := range cst.Imports {
		astImp := Import{Path: imp.PathTok.Lexeme}
		if imp.AliasTok != nil {
			astImp.As = imp.AliasTok.Lexeme
		}
		mod.Imports = append(mod.Imports, astImp)
	}

	// Convert constants
	if cst.Constants != nil {
		for _, entry := range cst.Constants.Entries {
			val, err := convertConstValue(entry.Val)
			if err != nil {
				return nil, err
			}
			mod.Constants = append(mod.Constants, Constant{
				Name:  entry.Key.Lexeme,
				Value: val,
			})
		}
	}

	// Convert actions to a single workflow
	if len(cst.Actions) > 0 {
		workflow, err := buildWorkflowFromActions(cst.Actions, cst)
		if err != nil {
			return nil, err
		}
		mod.Workflows = append(mod.Workflows, *workflow)
	}

	return mod, nil
}

// buildParallelStatePair lowers an SWSL action carrying [count: N] fork
// attributes into a parallel-fork state immediately followed by a synthetic
// wait (join) state — SWSL's sugar for the full-WSL pair:
//
//	parallel[count: N] <baseName> { action ...; on success -> <baseName>_join }
//	wait <baseName>_join { join <baseName> }
//
// Unlike full WSL (where fork and wait can be separated by other states so
// work overlaps with the branches), SWSL's single linear chain has no syntax
// for states in between, so the join is always immediate: the chain's next
// action/terminal only runs after every branch completes. Returns the fork
// state's name (the entry point other transitions should target) and the
// wait state's name (the state that should receive the chain's continuation
// — success/error transitions or a terminal end).
func buildParallelStatePair(action *SimplifiedCSTAction, baseName string, wf *Workflow) (forkName, waitName string, err error) {
	count, err := parallelCount(action.ParallelAttrs)
	if err != nil {
		return "", "", fmt.Errorf("state '%s': %w", baseName, err)
	}

	forkName = baseName
	waitName = baseName + "_join"
	if _, exists := wf.States[forkName]; exists {
		return "", "", fmt.Errorf("duplicate state '%s'", forkName)
	}
	if _, exists := wf.States[waitName]; exists {
		return "", "", fmt.Errorf("duplicate state '%s'", waitName)
	}

	act := &Action{Name: action.QNameTok.Lexeme}
	if idx := findLastDotOrSlash(action.QNameTok.Lexeme); idx != -1 {
		act.Module = action.QNameTok.Lexeme[:idx]
		act.Name = action.QNameTok.Lexeme[idx+1:]
	}
	for _, arg := range action.Args {
		act.Args = append(act.Args, Expr{Raw: arg.Raw})
	}
	if action.AliasTok != nil {
		act.As = action.AliasTok.Lexeme
	}

	wf.States[forkName] = &State{
		Name:          forkName,
		Action:        act,
		Parallel:      true,
		ParallelCount: count,
		Transitions: []Transition{
			{Name: "fork_join", Condition: Condition{Kind: CondSuccess}, Target: waitName},
		},
	}
	wf.States[waitName] = &State{
		Name:       waitName,
		Wait:       true,
		JoinTarget: forkName,
	}

	return forkName, waitName, nil
}

// buildWorkflowFromActions converts simplified actions into a workflow
func buildWorkflowFromActions(actions []SimplifiedCSTAction, cst *SimplifiedCSTModule) (*Workflow, error) {
	// Determine workflow name
	// Default is "main", unless explicitly specified with workflow type declaration
	name := "main"
	if cst.WorkflowName != nil {
		// Use explicit workflow name if provided
		name = cst.WorkflowName.Lexeme
	}

	// Determine workflow type
	workflowType := "workflow" // default
	if cst.WorkflowType != nil {
		workflowType = cst.WorkflowType.Lexeme
	}

	wf := &Workflow{
		Name:   name,
		Type:   workflowType,
		States: make(map[string]*State),
	}

	if len(actions) == 0 {
		return wf, nil
	}

	// Separate def actions (definitions) from regular actions
	definitions := make(map[string]*SimplifiedCSTAction)
	regularActions := []SimplifiedCSTAction{}

	for i := range actions {
		action := &actions[i]
		if action.DefTok != nil {
			// This is a definition, store it by alias
			if action.AliasTok != nil {
				definitions[action.AliasTok.Lexeme] = action
			}
			// Don't add to regular actions - definitions don't create states
		} else {
			regularActions = append(regularActions, *action)
		}
	}

	if len(regularActions) == 0 {
		// No regular actions, only definitions
		return wf, nil
	}

	// First pass: create all states and assign names for regular actions only.
	// entryNames[i] is the state a predecessor should transition to (the fork
	// state for a parallel action, otherwise the action's own state).
	// wireNames[i] is the state that owns this action's outgoing transitions
	// (the synthetic join state for a parallel action, since success/error
	// there means "all branches finished"; otherwise the action's own state).
	stateCounter := 0
	entryNames := make(map[int]string)
	wireNames := make(map[int]string)

	for i, action := range regularActions {
		if len(action.ParallelAttrs) > 0 {
			baseName := action.QNameTok.Lexeme
			if action.AliasTok != nil {
				baseName = action.AliasTok.Lexeme
			} else {
				baseName = generateStateName(baseName, stateCounter)
				stateCounter++
			}
			forkName, waitName, err := buildParallelStatePair(&action, baseName, wf)
			if err != nil {
				return nil, err
			}
			entryNames[i] = forkName
			wireNames[i] = waitName
			continue
		}

		var stateName string
		if action.AliasTok != nil {
			stateName = action.AliasTok.Lexeme
		} else {
			// Generate state name from action
			stateName = generateStateName(action.QNameTok.Lexeme, stateCounter)
			stateCounter++
		}
		entryNames[i] = stateName
		wireNames[i] = stateName

		// Create basic state
		state := &State{
			Name: stateName,
		}

		// Build action
		act := &Action{
			Name: action.QNameTok.Lexeme,
		}

		// Split qualified name if present
		if idx := findLastDotOrSlash(action.QNameTok.Lexeme); idx != -1 {
			act.Module = action.QNameTok.Lexeme[:idx]
			act.Name = action.QNameTok.Lexeme[idx+1:]
		}

		// Parse arguments
		for _, arg := range action.Args {
			act.Args = append(act.Args, Expr{Raw: arg.Raw})
		}

		if action.AliasTok != nil {
			act.As = action.AliasTok.Lexeme
		}

		state.Action = act
		wf.States[stateName] = state
	}

	// Set start state
	wf.Start = entryNames[0]
	wf.States[entryNames[0]].Start = true

	// Second pass: wire up transitions and flows
	for i, action := range regularActions {
		state := wf.States[wireNames[i]]
		if err := wireStateTransitions(&action, state, wf, &stateCounter, definitions, "ok"); err != nil {
			return nil, err
		}
	}

	return wf, nil
}

// addActionFlowToWorkflow recursively adds an action and its flows to the workflow.
// isErrorHandler controls whether a terminal (-> .) on this action ends with kind "fail"
// (error path) or "ok" (success path).
//
// If action carries [count: N] fork attributes, it lowers to a parallel-fork
// state plus a synthetic join state (see buildParallelStatePair); the
// returned name is the fork state (what a predecessor should transition to),
// and the action's own error/next flow is wired onto the join state instead,
// so success/error only fires after every branch completes.
func addActionFlowToWorkflow(action *SimplifiedCSTAction, wf *Workflow, stateCounter *int, definitions map[string]*SimplifiedCSTAction, isErrorHandler bool) (string, error) {
	terminalKind := "ok"
	if isErrorHandler {
		terminalKind = "fail"
	}

	if len(action.ParallelAttrs) > 0 {
		baseName := action.QNameTok.Lexeme
		if action.AliasTok != nil {
			baseName = action.AliasTok.Lexeme
		} else {
			baseName = generateStateName(baseName, *stateCounter)
			*stateCounter++
		}
		// If this exact def action was already inlined elsewhere (reused via
		// an alias reference), reuse its fork/join pair instead of rebuilding.
		if _, exists := wf.States[baseName]; exists {
			return baseName, nil
		}
		forkName, waitName, err := buildParallelStatePair(action, baseName, wf)
		if err != nil {
			return "", err
		}
		if err := wireStateTransitions(action, wf.States[waitName], wf, stateCounter, definitions, terminalKind); err != nil {
			return "", err
		}
		return forkName, nil
	}

	// Determine state name
	var stateName string
	if action.AliasTok != nil {
		stateName = action.AliasTok.Lexeme
	} else {
		stateName = generateStateName(action.QNameTok.Lexeme, *stateCounter)
		*stateCounter++
	}

	// Check if state already exists
	if _, exists := wf.States[stateName]; exists {
		return stateName, nil
	}

	// Create state
	state := &State{
		Name: stateName,
	}

	// Build action
	act := &Action{
		Name: action.QNameTok.Lexeme,
	}

	// Split qualified name if present
	if idx := findLastDotOrSlash(action.QNameTok.Lexeme); idx != -1 {
		act.Module = action.QNameTok.Lexeme[:idx]
		act.Name = action.QNameTok.Lexeme[idx+1:]
	}

	// Parse arguments
	for _, arg := range action.Args {
		act.Args = append(act.Args, Expr{Raw: arg.Raw})
	}

	if action.AliasTok != nil {
		act.As = action.AliasTok.Lexeme
	}

	state.Action = act

	if err := wireStateTransitions(action, state, wf, stateCounter, definitions, terminalKind); err != nil {
		return "", err
	}

	wf.States[stateName] = state
	return stateName, nil
}

// wireStateTransitions wires error bindings and next-flow transitions onto state from action.
// terminalKind ("ok" or "fail") is used when a -> . terminal is encountered.
func wireStateTransitions(action *SimplifiedCSTAction, state *State, wf *Workflow, stateCounter *int, definitions map[string]*SimplifiedCSTAction, terminalKind string) error {
	// Handle error binding reference
	if action.ErrorBindingRef != nil {
		refName := action.ErrorBindingRef.Lexeme
		if defAction, isDef := definitions[refName]; isDef {
			// Inline the definition as an error handler state
			errorStateName, err := addActionFlowToWorkflow(defAction, wf, stateCounter, definitions, true)
			if err != nil {
				return err
			}
			state.Transitions = append(state.Transitions, Transition{
				Name:      "error_handler",
				Condition: Condition{Kind: CondError},
				Target:    errorStateName,
			})
		} else {
			state.Transitions = append(state.Transitions, Transition{
				Name:      "error_handler",
				Condition: Condition{Kind: CondError},
				Target:    refName,
			})
		}
	}

	// Handle inline error binding action
	if action.ErrorBindingAction != nil {
		errorStateName, err := addActionFlowToWorkflow(action.ErrorBindingAction, wf, stateCounter, definitions, true)
		if err != nil {
			return err
		}
		state.Transitions = append(state.Transitions, Transition{
			Name:      "error_handler",
			Condition: Condition{Kind: CondError},
			Target:    errorStateName,
		})
	}

	// Handle next flow
	if action.NextFlow != nil {
		if action.NextFlow.Terminal {
			state.End = &End{Kind: terminalKind}
		} else if action.NextFlow.Ref != nil {
			// Reference to a def alias via -> (success binding)
			refName := action.NextFlow.Ref.Lexeme
			if defAction, isDef := definitions[refName]; isDef {
				successStateName, err := addActionFlowToWorkflow(defAction, wf, stateCounter, definitions, false)
				if err != nil {
					return err
				}
				state.Transitions = append(state.Transitions, Transition{
					Name:      "next",
					Condition: Condition{Kind: CondSuccess},
					Target:    successStateName,
				})
			} else {
				state.Transitions = append(state.Transitions, Transition{
					Name:      "next",
					Condition: Condition{Kind: CondSuccess},
					Target:    refName,
				})
			}
		} else if action.NextFlow.Target != nil {
			nextStateName, err := addActionFlowToWorkflow(action.NextFlow.Target, wf, stateCounter, definitions, false)
			if err != nil {
				return err
			}
			state.Transitions = append(state.Transitions, Transition{
				Name:      "next",
				Condition: Condition{Kind: CondSuccess},
				Target:    nextStateName,
			})
		}
	}

	return nil
}

// buildStateFromAction converts a SimplifiedCSTAction to a State
// Helper functions
func generateStateName(actionName string, counter int) string {
	// Generate a state name from action name
	if counter == 0 {
		return actionName
	}
	return actionName + "_" + strconv.Itoa(counter)
}

func findLastDotOrSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' || s[i] == '/' {
			return i
		}
	}
	return -1
}
