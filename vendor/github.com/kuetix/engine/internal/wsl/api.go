package wsl

// ParseAll is a convenience pipeline:
// src (WSL text) -> CST -> AST -> Graphs
// Returns AST Module and per-workflow Graphs.
func ParseAll(src string, name string) (*Module, map[string]*Graph, error) {
	cst, err := ParseCST(src, name)
	if err != nil {
		return nil, nil, err
	}
	ast, err := BuildAST(cst)
	if err != nil {
		return nil, nil, err
	}
	graphs := BuildGraphs(ast)
	return ast, graphs, nil
}

// ParseAllSimplified is a convenience pipeline for SimplifiedWSL:
// src (SimplifiedWSL text) -> AST -> Graphs
// Returns AST Module and per-workflow Graphs.
func ParseAllSimplified(src string, name string) (*Module, map[string]*Graph, error) {
	return ParseAllSimplifiedWithFilename(src, name)
}

// ParseAllSimplifiedWithFilename is a convenience pipeline for SimplifiedWSL with filename support:
// src (SimplifiedWSL text) -> AST -> Graphs
// If filename is provided and module keyword is missing, derives module name from filename
// Returns AST Module and per-workflow Graphs.
func ParseAllSimplifiedWithFilename(src string, filename string) (*Module, map[string]*Graph, error) {
	ast, err := ParseSimplifiedWSLWithFilename(src, filename)
	if err != nil {
		return nil, nil, err
	}
	graphs := BuildGraphs(ast)
	return ast, graphs, nil
}
