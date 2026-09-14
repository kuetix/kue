package workflow

import (
	baseContext "context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

type WorkerSessionContext struct {
	Engine          EngineInterface
	Flow            *domain.Flow
	Worker          Worker
	WorkflowContext WorkerContext
	ServerContext   baseContext.Context
	Parser          *Parser
}

//goland:noinspection GoUnusedExportedFunction
func NewWorkerSessionContext(engine EngineInterface, flow *domain.Flow, worker Worker, workflowContext WorkerContext, serverContext baseContext.Context) *WorkerSessionContext {
	return &WorkerSessionContext{
		Engine:          engine,
		Flow:            flow,
		Worker:          worker,
		WorkflowContext: workflowContext,
		ServerContext:   serverContext,
		Parser:          NewParser(),
	}
}

func (p *WorkerSessionContext) StartStep() *WorkerSessionContext {
	p.UpdateContext()
	return p
}

func (p *WorkerSessionContext) GetLookupContext() (context map[string]interface{}) {
	context = *p.WorkflowContext.Context()
	options := domain.FlowOptionsPtrKey(&context, "options")
	optionsMap := domain.GetAllOptions(p.Flow)
	options = &optionsMap
	context["options"] = options

	return context
}

func (p *WorkerSessionContext) GetProperties(props ...string) (context *map[string]interface{}, values map[string]interface{}) {
	values = make(map[string]interface{})
	if len(props) > 0 {
		for _, prop := range props {
			key, value, err := p.GetProperty(prop)
			if !p.Worker.HandleError(err, http.StatusBadRequest) {
				return nil, nil
			}
			values[key] = value
		}
	}

	return p.WorkflowContext.Context(), values
}

func (p *WorkerSessionContext) UpdateContext() (options *map[string]interface{}) {
	context := p.WorkflowContext.Context()
	(*context)["options"] = p.Flow.AllOptions()
	return context
}

func (p *WorkerSessionContext) GetContextWithCheck(key string) (interface{}, bool) {
	i, ok := (*p.WorkflowContext.Context())[key]
	if ok {
		return i, true
	}

	return nil, false
}

func (p *WorkerSessionContext) Context(key string) interface{} {
	context, _ := p.GetContextWithCheck(key)

	return context
}

func (p *WorkerSessionContext) SetContext(key string, value interface{}) *WorkerSessionContext {
	(*p.WorkflowContext.Context())[key] = value

	return p
}

func (p *WorkerSessionContext) String(key string, byDefault ...string) string {
	i, ok := p.GetPropertyWithCheck(key)
	if ok {
		return i.(string)
	}

	if len(byDefault) > 0 {
		return byDefault[0]
	}

	return ""
}

func (p *WorkerSessionContext) Int(key string, byDefault ...int) (value int, ok bool) {
	value, isType := helpers.MustInt(key, byDefault...)
	if isType == "" {
		p.Worker.HandleError(fmt.Errorf("%s is not a number", key), http.StatusBadRequest)
		return 0, false
	}
	return value, true
}

func (p *WorkerSessionContext) GetIntWithCheck(key string) (int, bool) {
	i, ok := p.GetPropertyWithCheck(key)
	return i.(int), ok
}

func (p *WorkerSessionContext) KeyExists(key string) bool {
	context := p.WorkflowContext.Context()

	values := helpers.MapKey(context, "values")
	if _, ok := values[key]; ok {
		return ok
	}

	options := domain.FlowOptionsPtrKey(context, "options")
	_, ok := domain.FlowOptionsKey(options, key, domain.OrderOptionsSearch, domain.OrderSubOptionsSearch)
	if ok {
		return ok
	}

	if _, ok := (*context)[key]; ok {
		return ok
	}

	return false
}

func (p *WorkerSessionContext) Option(key string) interface{} {
	context := p.WorkflowContext.Context()
	options := domain.FlowOptionsPtrKey(context, "options")
	i, ok := domain.FlowOptionsKey(options, key, domain.OrderOptionsSearch, domain.OrderSubOptionsSearch)
	if ok {
		return i
	}

	return nil
}

func (p *WorkerSessionContext) Value(key string) interface{} {
	context := p.WorkflowContext.Context()

	values := helpers.MapKey(context, "values")
	if i, ok := values[key]; ok {
		return i
	}

	return nil
}

func (p *WorkerSessionContext) SetValue(key string, value interface{}) *WorkerSessionContext {
	values := helpers.MapKey(p.WorkflowContext.Context(), "values")
	if values == nil {
		values = map[string]interface{}{key: value}
	} else {
		values[key] = value
	}
	return p
}

func (p *WorkerSessionContext) RemoveValue(key string) *WorkerSessionContext {
	values := helpers.MapKey(p.WorkflowContext.Context(), "values")
	if _, ok := values[key]; ok {
		delete(values, key)
	}

	return p
}

func (p *WorkerSessionContext) LastReturn() interface{} {
	for _, key := range p.Flow.CurrentTransition.From {
		value, ok := p.GetPropertyWithCheck(fmt.Sprintf("return:%s", key))
		if ok {
			return value
		}
	}

	return nil
}

func (p *WorkerSessionContext) Return(value interface{}) *WorkerSessionContext {
	return p.SetValue(fmt.Sprintf("return:%s", p.Flow.CurrentTransition.To), value)
}

func (p *WorkerSessionContext) LookupProperty(key string) interface{} {
	p.UpdateContext()

	return p.Property(key)
}

func (p *WorkerSessionContext) Property(key string) interface{} {
	if strings.Contains(key, ".") {
		parts := strings.Split(key, ".")
		rootKey := parts[0]
		key = strings.Join(parts[1:], ".")
		rootValue := p.Property(rootKey)
		if rootValue == nil {
			return nil
		}
		if strings.Contains(rootKey, "[") {
			//goland:noinspection RegExpRedundantEscape
			regex := regexp.MustCompile(`\[(\d+)\]`)
			matches := regex.FindStringSubmatch(rootKey)
			if matches != nil && helpers.IsSlice(rootValue) {
				if len(matches) > 0 {
					if len(matches[0]) > 1 {
						no, err := strconv.Atoi(string(matches[0][1]))
						if err == nil {
							rootValue = rootValue.([]interface{})[no]
						}
					}
				}
			}
		}
		return p.recursiveGetValue(rootValue, key)
	} else {
		return p.Lookup(key)
	}
}

func (p *WorkerSessionContext) recursiveGetValue(rootValue interface{}, key string) interface{} {
	var val interface{}
	regx := regexp.MustCompile(`\d+`)
	rootVal := rootValue
	if helpers.IsStruct(rootValue) {
		var err error
		rootVal, err = helpers.ToMapRecursive(rootValue)
		if err != nil {
			rootVal = rootValue
		}
	}
	if v, ok := rootVal.(map[string]interface{}); ok {
		key = strings.TrimSpace(key)
		if strings.Contains(key, ".") {
			parts := strings.Split(key, ".")
			rootKey := strings.ReplaceAll(parts[0], " ", "")
			if strings.Contains(rootKey, "[") {
				splits := strings.Split(rootKey, "[")
				for _, split := range splits {
					k := strings.TrimSpace(strings.Trim(split, "]"))
					if regx.MatchString(k) {
						no, err := strconv.Atoi(k)
						if err != nil {
							return nil
						}
						if helpers.IsSlice(rootValue) && no < len(rootValue.([]interface{})) {
							rootValue = rootValue.([]interface{})[no]
						}
					} else {
						val, ok = rootValue.(map[string]interface{})[k]
						if ok {
							rootValue = val
						}
					}
				}
			}
			key = strings.Join(parts[1:], ".")
			if childValue, ok := v[rootKey]; ok {
				rootValue = childValue
			}

			return p.recursiveGetValue(rootValue, key)
		}
		if value, ok := v[key]; ok {
			return value
		} else {
			return nil
		}
	}

	return nil
}

func (p *WorkerSessionContext) Lookup(key string) interface{} {
	context := p.WorkflowContext.Context()

	values := helpers.MapKey(context, "values")
	if i, ok := values[key]; ok {
		return i
	}

	options := domain.FlowOptionsPtrKey(context, "options")
	if i, ok := domain.FlowOptionsKey(options, key, domain.OrderOptionsSearch, domain.OrderSubOptionsSearch); ok {
		return i
	}

	if i, ok := (*context)[key]; ok {
		return i
	}

	optionsMap := domain.GetAllOptions(p.Flow)
	options = &optionsMap
	i, ok := domain.FlowOptionsKey(options, key, domain.OrderOptionsSearch, domain.OrderSubOptionsSearch)
	if ok {
		return i
	}

	if i, ok = (*context)[key]; ok {
		return i
	}

	return key
}

func (p *WorkerSessionContext) GetRecursivePropertyWithCheck(key interface{}) (interface{}, bool) {
	switch v := key.(type) {
	case string:
		value, ok := p.GetPropertyWithCheck(v)
		if !ok {
			return value, ok
		}
		switch value.(type) {
		case map[string]interface{}, []interface{}:
			return p.GetRecursivePropertyWithCheck(value)
		default:
			return value, true
		}
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			newVal, ok := p.GetRecursivePropertyWithCheck(val)
			if !ok {
				return nil, false
			}
			result[k] = newVal
		}
		return result, true
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			newVal, ok := p.GetRecursivePropertyWithCheck(val)
			if !ok {
				return nil, false
			}
			result[i] = newVal
		}
		return result, true
	default:
		return key, true
	}
}

func (p *WorkerSessionContext) GetPropertyWithCheck(key string) (interface{}, bool) {
	context := p.WorkflowContext.Context()

	values := helpers.MapKey(context, "values")
	if i, ok := values[key]; ok {
		return i, true
	}

	options := domain.GetAllOptions(p.Flow)
	i, ok := domain.FlowOptionsKey(&options, key, domain.OrderOptionsSearch, domain.OrderSubOptionsSearch)
	if ok {
		return i, true
	}

	if i, ok := (*context)[key]; ok {
		return i, true
	}

	return nil, false
}

func (p *WorkerSessionContext) GetPropertyAsInt(key string, defaultValue int) int {
	value := defaultValue
	valueIf, ok := p.GetPropertyWithCheck(key)
	if !ok {
		valueInt, err := strconv.Atoi(valueIf.(string))
		if err != nil {
			value = defaultValue
		} else {
			value = valueInt
		}
	}

	return value
}

func (p *WorkerSessionContext) GetPropertyAsInt64(key string, defaultValue int64) int64 {
	value := defaultValue
	valueIf, ok := p.GetPropertyWithCheck(key)
	if !ok {
		if valueIf == nil {
			value = defaultValue
		} else {
			valueInt, err := strconv.Atoi(valueIf.(string))
			if err != nil {
				value = defaultValue
			} else {
				value = int64(valueInt)
			}
		}
	}

	return value
}

func (p *WorkerSessionContext) RecursiveParseProperty(prop interface{}) (key string, value interface{}, err error) {
	var parsedVal interface{}
	switch v := prop.(type) {
	case string:
		return p.ParseProperty(v)
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			key, parsedVal, err = p.RecursiveParseProperty(val)
			if err != nil {
				return key, nil, err
			}
			result[k] = parsedVal
		}
		return "", result, nil
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			key, parsedVal, err = p.RecursiveParseProperty(val)
			if err != nil {
				return key, nil, err
			}
			result[i] = parsedVal
		}
		return "", result, nil
	default:
		return "", prop, nil
	}
}

func (p *WorkerSessionContext) ParseProperty(prop string) (key string, value interface{}, err error) {
	matchContent := prop
	matched := false
	if p.Parser.isTriangleBrackets(prop) {
		//goland:noinspection RegExpDuplicateCharacterInClass
		valuesRe := regexp.MustCompile(`<<([^<<].*?)>>`) // Matches functionName(arg1, arg2)
		matches := valuesRe.FindAllStringSubmatch(prop, -1)
		if len(matches) == 0 {
			return "", nil, fmt.Errorf("ParseProperty can't find: %s", prop)
		}

		if len(matches[0]) < 2 {
			return "", nil, fmt.Errorf("invalid property pattern format in: %s", prop)
		}

		matched = true
		matchContent = matches[0][1]
	}
	property, v, err := p.GetProperty(matchContent)
	if v == matchContent && matched {
		return property, v, fmt.Errorf("property %q not found, from expr: %q", matchContent, prop)
	}
	return property, v, err
}

func (p *WorkerSessionContext) GetProperty(prop string) (key string, value interface{}, err error) {
	return p.Parser.GetProperty(prop, p.GetPropertyFromContext)
}

func (p *WorkerSessionContext) GetPropertyFromContext(prop string) (key string, value interface{}, err error) {
	logger.Debugf("GetProperty: %s", prop)
	placeholder := "__DOUBLE_PIPE__"
	prop = strings.Trim(prop, "\"' ")
	prop = strings.ReplaceAll(prop, "||", placeholder)
	// Split using a single '|'
	defs := strings.Split(prop, "??")
	var defaultValue string
	if len(defs) > 1 {
		prop = strings.Trim(defs[0], "\"' ")
		defaultValue = strings.Trim(defs[1], "\"' ")
	}
	property := strings.Split(prop, "|")
	// Replace the placeholder back to "||"
	for i, part := range property {
		property[i] = strings.TrimSpace(strings.ReplaceAll(part, placeholder, "|"))
	}
	key = property[0]
	logger.Debugf("GetProperty: key: %s, property: %v", key, property)
	value = p.Property(key)
	logger.Debugf("GetProperty: initial value: %v", value)
	if len(property) < 2 || value == nil {
		if defaultValue != "" {
			property = strings.Split(defaultValue, "|")
			// Replace the placeholder back to "||"
			for i, part := range property {
				property[i] = strings.TrimSpace(strings.ReplaceAll(part, placeholder, "|"))
			}
			value = property[0]
			property[0] = key
			// value = defaultValue
		} else {
			return key, value, nil
		}
	}
	for i := 1; i < len(property); i++ {
		switch property[i] {
		case "int":
			value, _ = helpers.MustInt(value)
			break
		case "string":
			value, _ = helpers.MustString(value)
			break
		case "bool":
			value, _ = helpers.MustBool(value)
			break
		case "pint":
			value, _ = p.ParsePropertyToInt(key)
			break
		case "toArray":
			value, _ = helpers.MustArray(value)
			break
		case "pstring":
			value, _ = p.ParsePropertyToString(key)
			break
		case "option":
			value = p.Option(key)
			break
		case "strings":
			value, _ = p.ConvertToStringSlice(key)
			break
		case "parse":
			value, _ = helpers.MustString(value)
			value, err = p.Parser.ParseTemplate(value.(string))
			if err != nil {
				return key, nil, err
			}
			break
		case "property":
			_, ok := value.(string)
			if ok {
				value = p.Property(value.(string))
			}
			break
		}
	}

	return key, value, nil
}

func (p *WorkerSessionContext) ProcessPropertiesRecursively(entity map[string]interface{}) (err error) {
	for key, value := range entity {
		switch value.(type) {
		case map[string]interface{}:
			err = p.ProcessPropertiesRecursively(value.(map[string]interface{}))
			if err != nil {
				return err
			}
		case string:
			_, result, err := p.GetProperty(value.(string))
			if err != nil {
				return err
			}
			entity[key] = result
		}
	}

	return nil
}

func (p *WorkerSessionContext) GetPropertyValue(prop string) (key string, value interface{}, err error) {
	placeholder := "__DOUBLE_PIPE__"
	prop = strings.ReplaceAll(prop, "||", placeholder)
	// Split using a single '|'
	property := strings.Split(prop, "|")
	// Replace the placeholder back to "||"
	for i, part := range property {
		property[i] = strings.ReplaceAll(part, placeholder, "|")
	}
	key = property[0]
	value = key
	if len(property) < 2 || value == nil {
		return key, value, nil
	}
	for i := 1; i < len(property); i++ {
		switch property[i] {
		case "int":
			value, _ = helpers.MustInt(value)
			break
		case "string":
			value, _ = helpers.MustString(value)
			break
		case "bool":
			value, _ = helpers.MustBool(value)
			break
		case "pint":
			value, _ = p.ParsePropertyToInt(key)
			break
		case "pstring":
			value, _ = p.ParsePropertyToString(key)
			break
		case "strings":
			value, _ = p.ConvertToStringSlice(key)
			break
		case "parse":
			value, _ = helpers.MustString(value)
			value, err = p.Parser.ParseTemplate(value.(string))
			if err != nil {
				return key, nil, err
			}
			break
		case "property":
			_, ok := value.(string)
			if ok {
				value = p.Property(value.(string))
			}
			break
		}
	}

	return key, value, nil
}

func (p *WorkerSessionContext) Bool(key string) bool {
	i, ok := p.GetPropertyWithCheck(key)
	if ok {
		return i.(bool)
	}

	return false
}

func (p *WorkerSessionContext) ParsePropertyToInt(key string) (int, bool) {
	value := p.Property(key)
	var result int
	if helpers.IsNumeric(value) {
		result, _ = helpers.MustInt(value)
	} else {
		resultString, err := p.Parser.ParseTemplate(value.(string))
		if !p.Worker.HandleError(err, http.StatusBadRequest) {
			return 0, false
		}
		result, _ = helpers.MustInt(resultString)
	}

	return result, true
}

func (p *WorkerSessionContext) ParsePropertyToString(key string) (string, bool) {
	value := p.Property(key)
	var valueString string
	if !helpers.IsString(value) {
		valueString, _ = helpers.MustString(value)
	} else {
		valueString = value.(string)
	}

	resultString, err := p.Parser.ParseTemplate(valueString)
	if !p.Worker.HandleError(err, http.StatusBadRequest) {
		return "", false
	}
	result, _ := helpers.MustString(resultString)

	return result, true
}

func (p *WorkerSessionContext) ConvertToStringSlice(key string) ([]string, bool) {
	value := p.Property(key)
	var valueString string
	if !helpers.IsString(value) {
		valueString, _ = helpers.MustString(value)
	} else {
		valueString = value.(string)
	}

	resultString, err := p.Parser.ParseTemplate(valueString)
	if !p.Worker.HandleError(err, http.StatusBadRequest) {
		return nil, false
	}
	result, _ := helpers.MustString(resultString)

	results, is := p.GetPropertyWithCheck(result)
	if !is {
		return nil, false
	}

	return results.([]string), true
}

func (p *WorkerSessionContext) ExprLookup(argName string) (value interface{}, err error) {
	argName = strings.TrimSpace(argName)
	argName = strings.Trim(argName, "\" \t\n\r")
	if strings.HasPrefix(argName, "<<") {
		_, value, err = p.ParseProperty(argName)
		if err != nil {
			logger.Debugf("error getting property %q from WorkerSessionContext: %v", argName, p.GetLookupContext())
			return nil, fmt.Errorf("error getting property %q from WorkerSessionContext: %w", argName, err)
		}
	}
	if value != nil {
		if _, ok := value.(string); !ok {
			return value, nil
		}
		if value.(string) != argName {
			return value, nil
		}
	} else {
		value = argName
	}
	raw, found := p.GetPropertyWithCheck(value.(string))
	if !found {
		return nil, fmt.Errorf("arg %q not found in WorkerSessionContext context", argName)
	}
	value = raw
	if rawString, ok := raw.(string); ok {
		if !strings.HasPrefix(rawString, "<<") && !strings.HasPrefix(rawString, "$") {
			return raw, nil
		}
		_, value, err = p.ParseProperty(rawString)
		if err != nil {
			logger.Debugf("error getting property %q from WorkerSessionContext: %v", argName, p.GetLookupContext())
			return nil, fmt.Errorf("error getting property %q from WorkerSessionContext: %w", argName, err)
		}
	} else {
		_, value, err = p.RecursiveParseProperty(raw)
		if err != nil {
			logger.Debugf("error getting property %q from WorkerSessionContext: %v", argName, p.GetLookupContext())
			return nil, fmt.Errorf("error getting property %q from WorkerSessionContext: %w", argName, err)
		}
	}
	return value, nil
}
