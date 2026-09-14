package workflow

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

type Node struct {
	Op    string  `json:"op,omitempty"`
	Parts []*Node `json:"parts,omitempty"`
	Value string  `json:"value,omitempty"`
}

type Parser struct {
}

const tokenSegmentClass = `[\w:~!@#$%^&*+/=_?\-]+`

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Property(key string) interface{} {
	return p.Property(key)
}

// ParseTemplate processes a template, replacing tokens with context values or invoking functions from context
func (p *Parser) ParseTemplate(template string) (string, error) {
	// Regular expression to match function calls and tokens (Corrected regex)
	//goland:noinspection RegExpDuplicateCharacterInClass
	valuesRe := regexp.MustCompile(`<<([^<<].*?)>>`) // Matches functionName(arg1, arg2)
	funcRe := regexp.MustCompile(`([\w.]+)\((.*)\)`) // Matches functionName(arg1, arg2)
	// tokenRe := regexp.MustCompile(`<<([\w:#]+)((\.[\w:#]+)*)>>`) // Matches <<entity>> or <<entity.field>>
	tokenRe := regexp.MustCompile(`<<(` + tokenSegmentClass + `)((\.` + tokenSegmentClass + `)*)((\?\?)(.*))?>>`) // Matches <<entity>> or <<entity.field>> including special vars like <<?>>

	result := valuesRe.ReplaceAllStringFunc(template, func(token string) string {
		if strings.Contains(token, "|") {
			token = strings.ReplaceAll(token, "|", ">>|<<")
		}
		tokens := strings.Split(strings.Trim(token, "{}"), "|")
		var result string
		length := len(tokens)
		for ti, t := range tokens {
			// Check if the template contains a function call
			if funcMatches := funcRe.FindStringSubmatch(t); len(funcMatches) == 3 {
				functionName := funcMatches[1]
				rawArgs := funcMatches[2]
				args := p.parseArgs(rawArgs)
				var arguments = make([]interface{}, len(args))

				// Resolve tokens inside arguments
				for i, arg := range args {
					if strings.Index(arg, "&") != 0 {
						resolved, err := p.resolveToken(fmt.Sprintf("<<%s>>", arg), tokenRe)
						if err != nil {
							return "" // Leave the token unchanged if there's an error
						}
						arguments[i] = resolved
					} else {
						ptrArg, found := p.Property(arg[1:]).(interface{})
						if found {
							arguments[i] = ptrArg
						}
					}
				}

				// Dynamically call the function from the context
				result, err := p.callFunction(functionName, arguments)
				if err != nil {
					logger.Error(fmt.Sprintf("Error calling function '%s': %s", functionName, err))
					return "" // Leave the token unchanged if there's an error
				}
				return result
			}

			var err error
			var resolved string
			var tokenized string
			tokenized = t
			if !p.isTriangleBrackets(t) {
				tokenized = fmt.Sprintf("<<%s>>", t)
			}
			// ChannelID function call, just replace tokens
			result = tokenRe.ReplaceAllStringFunc(tokenized, func(token string) string {
				resolved, err = p.resolveToken(token, tokenRe)
				if err != nil {
					return tokenized // Leave the token unchanged if there's an error
				}
				return resolved
			})

			if result != token && result != template && result != "" && !p.isTriangleBrackets(result) {
				return result
			} else if err == nil && !p.isTriangleBrackets(t) {
				parseTemplate, err := p.ParseTemplate(result)
				if err != nil {
					return result
				}
				return parseTemplate
			} else {
				if ti >= length-1 {
					return result
				}
			}
		}

		return result
	})

	if result == template {
		return result, nil
	}

	if !p.isTriangleBrackets(result) {
		result = removeSlashesFromCurlyBrackets(result)
		return result, nil
	}

	return p.ParseTemplate(result)
}

func (p *Parser) isTriangleBrackets(template string) bool {
	template = strings.ReplaceAll(template, `\<<`, "")
	template = strings.ReplaceAll(template, `\>>`, "")
	return strings.Contains(template, "<<") && strings.Contains(template, ">>")
}

// parseArgs splits the raw arguments of a function into individual tokens
func (p *Parser) parseArgs(rawArgs string) []string {
	return strings.Split(strings.TrimSpace(rawArgs), ", ")
}

// resolveToken resolves a token like {entity.field} from the context map
func (p *Parser) resolveToken(token string, tokenRe *regexp.Regexp) (string, error) {
	var defaultValue *string
	// Match the token pattern
	matches := tokenRe.FindStringSubmatch(token)
	if len(matches) < 2 {
		return "", fmt.Errorf("invalid token format: %s", token)
	}

	if len(matches) > 6 && matches[5] == "??" {
		defaultValue = &matches[6]
	}

	// Get the entity and field path
	entityName := matches[1]
	fieldPath := strings.Split(matches[2], ".")[1:] // Ignore the first "."

	// Find the entity in the context
	entity, exists := p.Property(entityName).(interface{})
	if !exists {
		if defaultValue != nil {
			return *defaultValue, nil
		}
		return "", fmt.Errorf("entity '%s' not found in context", entityName)
	}

	// Resolve nested fields using recursion
	value, found := p.resolveNestedFields(entity, fieldPath)
	if !found {
		if defaultValue != nil {
			return *defaultValue, nil
		}
		return "", fmt.Errorf("field '%s' not found in entity '%s'", strings.Join(fieldPath, "."), entityName)
	}

	if defaultValue != nil {
		if helpers.IsEmptyValue(value) {
			return *defaultValue, nil
		}
	}

	return fmt.Sprintf("%v", value), nil
}

// resolveNestedFields uses recursion to resolve nested fields in a struct
func (p *Parser) resolveNestedFields(entity interface{}, fields []string) (interface{}, bool) {
	// EngineName case: if there are no more fields to resolve, return the entity
	if len(fields) == 0 {
		return entity, true
	}

	var field reflect.Value
	// Get the current value and resolve the first field in the list
	current := reflect.Indirect(reflect.ValueOf(entity))
	keyName := fields[0]
	if current.Kind() == reflect.Slice {
		// Access the field by name
		atoi, err := strconv.Atoi(keyName)
		if err != nil || atoi >= current.Len() {
			return nil, false
		}
		field = current.Index(atoi)
	}

	if current.Kind() == reflect.Struct {
		// Access the field by name
		field = current.FieldByName(keyName)
	}

	if current.Kind() == reflect.Map {
		// Access the field by name
		field = current.MapIndex(reflect.ValueOf(keyName))
	}

	if !field.IsValid() {
		return nil, false
	}

	// Recursively resolve the remaining fields
	return p.resolveNestedFields(field.Interface(), fields[1:])
}

// callFunction dynamically calls a function from the context using reflection
func (p *Parser) callFunction(functionName string, args []interface{}) (answer string, err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in callFunction", r)
			logger.Debug("Recovered in callFunction", r)
			err = fmt.Errorf("function '%s' got an error: %s", functionName, r)
		}
	}()

	fnContext, fnExists := p.Property(functionName).(interface{})
	functionNameDi := fmt.Sprintf("%s%s", defines.HelpersPrefix, functionName)
	diExists := di.CanResolve(functionNameDi)
	var fn interface{}
	if fnExists {
		fn = fnContext
	} else if diExists {
		fn = di.Resolve(functionNameDi).(interface{})
	}
	if !fnExists && !diExists {
		return "", fmt.Errorf("function '%s' not found in context", functionName)
	}

	fnValue := reflect.ValueOf(fn)
	if fnValue.Kind() != reflect.Func {
		return "", fmt.Errorf("context item '%s' is not a function", functionName)
	}

	isVariadic := fnValue.Type().IsVariadic()

	requiredInMin := fnValue.Type().NumIn()
	if isVariadic {
		requiredInMin--
		// If the function is variadic, the last argument can accept multiple values
		if len(args) < requiredInMin {
			return "", fmt.Errorf("expected %d arguments for function '%s', got %d", requiredInMin, functionName, len(args))
		}
	} else {
		if len(args) != requiredInMin {
			return "", fmt.Errorf("expected %d arguments for function '%s', got %d", requiredInMin, functionName, len(args))
		}
	}

	// Prepare arguments for the function call
	callArgs := make([]reflect.Value, len(args))
	for i, arg := range args {
		callArgs[i] = reflect.ValueOf(arg)
	}

	// Call the function and return the result
	results := fnValue.Call(callArgs)
	if len(results) > 0 {
		if results[0].Kind() == reflect.Slice {
			return addSlashesToCurlyBrackets(string(results[0].Bytes())), nil
		}
		return results[0].String(), nil
	}

	return "", nil
}

func (p *Parser) GetProperty(expr string, propertyFromContext func(prop string) (key string, value interface{}, err error)) (key string, value interface{}, err error) {
	tree := p.parse(expr)

	found := p.walk(tree, 0, func(n *Node, level int) bool {
		if n.Op != "" {
			return false
		}

		k, r, e := propertyFromContext(n.Value)
		if e != nil {
			value = nil
			key = ""
			err = e
			return false
		}

		if r != nil {
			key = k
			value = r
			return true
		}

		return false
	})

	if found && value != nil {
		return key, value, nil
	}

	return "", nil, nil
}

func (p *Parser) findValue(expr string) any {
	target := "$kon.username"
	if expr == target || expr == "'"+target+"'" || expr == `"`+target+`"` {
		return expr
	}

	return nil
}

func (p *Parser) walk(node *Node, level int, fn func(*Node, int) bool) bool {
	if node == nil {
		return false
	}

	if fn(node, level) {
		return true
	}

	for _, part := range node.Parts {
		if p.walk(part, level+1, fn) {
			return true
		}
	}

	return false
}

func (p *Parser) parse(expr string) *Node {
	expr = strings.TrimSpace(expr)
	expr = p.trimOuterParens(expr)

	if parts := p.splitTopLevel(expr, "??"); len(parts) > 1 {
		node := &Node{Op: "??"}
		for _, part := range parts {
			node.Parts = append(node.Parts, p.parse(part))
		}
		return node
	}

	if parts := p.splitTopLevel(expr, "||"); len(parts) > 1 {
		node := &Node{Op: "||"}
		for _, part := range parts {
			node.Parts = append(node.Parts, p.parse(part))
		}
		return node
	}

	return &Node{Value: expr}
}

func (p *Parser) splitTopLevel(s, op string) []string {
	var parts []string
	start := 0
	depth := 0
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '\'' && !inDouble {
			if i == 0 || s[i-1] != '\\' {
				inSingle = !inSingle
			}
			continue
		}

		if ch == '"' && !inSingle {
			if i == 0 || s[i-1] != '\\' {
				inDouble = !inDouble
			}
			continue
		}

		if inSingle || inDouble {
			continue
		}

		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		}

		if depth == 0 && i+len(op) <= len(s) && s[i:i+len(op)] == op {
			part := strings.TrimSpace(s[start:i])
			parts = append(parts, part)
			start = i + len(op)
			i += len(op) - 1
		}
	}

	if len(parts) == 0 {
		return nil
	}

	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

func (p *Parser) trimOuterParens(s string) string {
	for {
		s = strings.TrimSpace(s)
		if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
			return s
		}
		if !p.isWrappedByOuterParens(s) {
			return s
		}
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
}

func (p *Parser) isWrappedByOuterParens(s string) bool {
	depth := 0
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '\'' && !inDouble {
			if i == 0 || s[i-1] != '\\' {
				inSingle = !inSingle
			}
			continue
		}

		if ch == '"' && !inSingle {
			if i == 0 || s[i-1] != '\\' {
				inDouble = !inDouble
			}
			continue
		}

		if inSingle || inDouble {
			continue
		}

		if ch == '(' {
			depth++
		}

		if ch == ')' {
			depth--
			if depth == 0 && i != len(s)-1 {
				return false
			}
		}
	}

	return depth == 0
}

// addSlashesToCurlyBrackets Function to add slashes to curly brackets in a string
func addSlashesToCurlyBrackets(query string) string {
	return strings.ReplaceAll(strings.ReplaceAll(query, "{", "\\{"), "}", "\\}")
}

func removeSlashesFromCurlyBrackets(query string) string {
	return strings.ReplaceAll(strings.ReplaceAll(query, "\\{", "{"), "\\}", "}")
}
