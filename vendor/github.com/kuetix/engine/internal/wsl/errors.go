package wsl

import (
	"fmt"
)

// ParseError represents a syntactic or lexical error with position.
type ParseError struct {
	Pos Position
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at %d:%d: %s", e.Pos.Line, e.Pos.Col, e.Msg)
}

func errf(pos Position, format string, a ...interface{}) *ParseError {
	return &ParseError{Pos: pos, Msg: fmt.Sprintf(format, a...)}
}

// SemanticError represents validation issues when converting CST -> AST or AST -> IR.
type SemanticError struct {
	Msg string
}

func (e *SemanticError) Error() string { return e.Msg }
