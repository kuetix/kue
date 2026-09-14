package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

func CallTransitionByName(path string, workerSessionContext *WorkerSessionContext, transition *ServiceTransitionMapping, metaCache map[string]map[string]map[string]interfaces.FunctionMetadata) (results []reflect.Value, anError error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Recovered in CallTransitionByName for path %s: %v", path, r)
			logger.Debugf("Stack trace: %s", string(debug.Stack()))
			results = nil
			anError = errors.New(fmt.Sprintf("panic in CallTransitionByName for path %s: %v", path, r))
		}
	}()
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid path, expected at least transition/method: %s", path)
	}

	service := transition.ServiceName
	method := parts[len(parts)-1]
	transitionPath := strings.Join(parts[:len(parts)-1], "/") // everything before the last slash
	transitionPath = strings.TrimPrefix(transitionPath, service+"/")

	// Extract service name and the real implementation
	impl := reflect.ValueOf(transition.Impl)
	if !impl.IsValid() {
		return nil, fmt.Errorf("invalid underlying Impl for %q", transitionPath)
	}

	// Get method reflects
	methodVal := impl.MethodByName(method)
	if !methodVal.IsValid() {
		return nil, fmt.Errorf("method %q not found on transition %q", method, transitionPath)
	}

	// Metadata lookup
	meta, ok := metaCache[service][transitionPath][method]
	if !ok {
		// Metadata lookup
		join := filepath.Join(service, transitionPath)
		meta, ok = metaCache[service][join][method]
		if !ok {
			return nil, fmt.Errorf("metadata not found for %s/%s/%s", service, transitionPath, method)
		}
	}

	// Prepare inputs
	inputs, err := prepareInput(meta, workerSessionContext, methodVal)
	if err != nil {
		logger.Debugf("Error preparing inputs for %s/%s/%s: %s", service, transitionPath, method, err.Error())

		jsonMeta, errJson, errMap := helpers.DebugAsPrettyJsonToBytes(meta)
		logger.Debugf("meta: %s\njson error:%v\nfrom map error:%v", jsonMeta, errJson, errMap)

		lookupContext, errJson, errMap := helpers.DebugAsPrettyJsonToBytes(workerSessionContext.GetLookupContext())
		logger.Debugf("lookupContext: %s\njson error:%v\nfrom map error:%v", lookupContext, errJson, errMap)
		return nil, errors.New(fmt.Sprintf("error preparing inputs for %s/%s/%s: %s", service, transitionPath, method, err.Error()))
	}

	var inputDetails []string
	argsCount := len(meta.ArgNames)
	defer func() {
		if r := recover(); r != nil {
			logger.Panicf("Panic in CallTransitionByName for %s/%s/%s: %v \n \t\t\t%d %d %d", service, transitionPath, method, r, len(inputs), len(inputDetails), argsCount)
		}
	}()

	// Debug logging inputs
	inputsNames := make([]string, 0)
	for i, input := range inputs {
		if meta.ArgTypes[i] != "*ast.Ellipsis" {
			if !input.IsValid() {
				continue
			}
			if input == reflect.Zero(input.Type()) {
				if input.Type().Kind() != reflect.String {
					continue
				}
			}
			inputDetails = append(inputDetails, fmt.Sprintf("params[%d]: interface=%v, type=%v", i, input.Interface(), input.Type().String()))
		} else {
			inputDetails = append(inputDetails, fmt.Sprintf("(optional)params[%d]: interface=%v, type=%v", i, meta.ArgNames[i], meta.ArgTypes[i]))
		}
		inputsNames = append(inputsNames, meta.ArgNames[i])
	}
	logger.Debugf("Calling %s/%s/%s with inputs:\n%s", service, transitionPath, method, strings.Join(inputDetails, "\n"))

	if argsCount != len(inputs) {
		for _, t := range meta.ArgTypes {
			if t == "*ast.Ellipsis" {
				argsCount--
			}
		}
	}
	if argsCount != len(inputs) || argsCount != len(inputDetails) {
		noneNames := map[string]interface{}{}
		for i, name := range meta.ArgNames {
			found := false
			for _, inputName := range inputsNames {
				if name == inputName {
					found = true
					continue
				}
			}
			if !found {
				if meta.ArgTypes[i] == "*ast.Ellipsis" {
					continue
				}
				noneNames[name] = name
			}
		}
		noneNamesSlice := make([]string, 0, len(noneNames))
		for name := range noneNames {
			noneNamesSlice = append(noneNamesSlice, name)
		}
		jsonNoneNamesSlice, errJson, errMap := helpers.DebugAsPrettyJsonToBytes(noneNamesSlice)
		logger.Debugf("noneNamesSlice: %s\njson error:%v\nfrom map error:%v", jsonNoneNamesSlice, errJson, errMap)

		lookupContext, errJson, errMap := helpers.DebugAsPrettyJsonToBytes(workerSessionContext.GetLookupContext())
		logger.Debugf("lookupContext: %s\njson error:%v\nfrom map error:%v", lookupContext, errJson, errMap)
		return nil, fmt.Errorf("mismatched input lengths for %s/%s/%s: require amount=%d, prepared amount=%d, available amount=%d, missed arguments: %v", service, transitionPath, method, argsCount, len(inputs), len(inputDetails), noneNamesSlice)
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Recovered in callFunction for function with args %v: %v", meta.ArgNames, r)
			logger.Errorf("Stack trace: %s", string(debug.Stack()))
		}
	}()
	// Call the method
	results = methodVal.Call(inputs)

	return results, nil
}

func prepareInput(meta interfaces.FunctionMetadata, workerSessionContext *WorkerSessionContext, fn reflect.Value) ([]reflect.Value, error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Recovered in prepareInput for function with args %v: %v", meta.ArgNames, r)
			logger.Debugf("Stack trace: %s", string(debug.Stack()))
		}
	}()
	fnType := fn.Type()
	args := make([]reflect.Value, len(meta.ArgNames))

	for i, name := range meta.ArgNames {
		expectedType := fnType.In(i)

		if name == "p" && expectedType.String() == "*workflow.WorkerSessionContext" {
			args[i] = reflect.ValueOf(workerSessionContext)
		} else {
			val, err := PrepareInput(workerSessionContext, expectedType, name)
			if meta.ArgTypes[i] == "*ast.Ellipsis" && err != nil {
				args = slices.Clip(args[:i])
				continue
			}
			if err != nil {
				return nil, logger.SErrorf("arg[%d] (%s): %w", i, name, err)
			}
			args[i] = val
		}
	}

	return args, nil
}

func PrepareInput(workerSessionContext *WorkerSessionContext, expectedType reflect.Type, argName string) (reflect.Value, error) {
	var err error
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Recovered in PrepareInput on argument: \"%s\" of type \"%s\": %v\n", argName, expectedType.String(), r)
		}
	}()

	// Case 1: Input is a WorkerSessionContext
	wp := workerSessionContext
	if wp != nil {
		var value interface{}
		// Try to extract value by key from the context
		value, err = wp.ExprLookup(argName)
		if value == nil {
			return reflect.Value{}, err
		}
		return castInput(value, expectedType)
	}

	// Case 2: Input is a normal map/buffer
	inputMap := *workerSessionContext.WorkflowContext.Context()
	if inputMap != nil {
		val, ok := inputMap[argName]
		if !ok {
			return reflect.Value{}, fmt.Errorf("arg %q not found in buffer", argName)
		}
		if val == nil {
			return reflect.ValueOf(val), nil
		}
		return castInput(val, expectedType)
	}

	return reflect.Value{}, fmt.Errorf("unsupported input type: %T", workerSessionContext)
}

func castInput(val any, expectedType reflect.Type) (reflect.Value, error) {
	v := reflect.ValueOf(val)

	// Direct assignment
	if v.Type().AssignableTo(expectedType) {
		return v, nil
	}

	// Map to struct a pointer
	if (expectedType.Kind() == reflect.Ptr && expectedType.Elem().Kind() == reflect.Map) && v.Kind() == reflect.Map {
		newVal := reflect.New(expectedType.Elem()).Interface()
		bytes, err := json.Marshal(val)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("marshal error: %w", err)
		}
		if err := json.Unmarshal(bytes, newVal); err != nil {
			return reflect.Value{}, fmt.Errorf("unmarshal into %s failed: %w", expectedType, err)
		}
		return reflect.ValueOf(newVal), nil
	}

	// Convertible
	if v.Type().ConvertibleTo(expectedType) {
		return v.Convert(expectedType), nil
	}

	if expectedType.Kind() == reflect.Float32 ||
		expectedType.Kind() == reflect.Float64 ||
		expectedType.Kind() == reflect.Int ||
		expectedType.Kind() == reflect.Int8 ||
		expectedType.Kind() == reflect.Int16 ||
		expectedType.Kind() == reflect.Int32 ||
		expectedType.Kind() == reflect.Int64 ||
		expectedType.Kind() == reflect.Uint ||
		expectedType.Kind() == reflect.Uint8 ||
		expectedType.Kind() == reflect.Uint16 ||
		expectedType.Kind() == reflect.Uint32 ||
		expectedType.Kind() == reflect.Uint64 {
		return v, nil
	}

	if expectedType.Kind() == reflect.Bool {
		switch v.Kind() {
		case reflect.Bool:
			return v, nil
		case reflect.String:
			if strings.ToLower(v.String()) == "true" {
				return reflect.ValueOf(true), nil
			}

			if strings.ToLower(v.String()) == "false" {
				return reflect.ValueOf(false), nil
			}

			if strings.ToLower(val.(string)) == "true" {
				return reflect.ValueOf(true), nil
			}

			if strings.ToLower(val.(string)) == "false" {
				return reflect.ValueOf(false), nil
			}

			return reflect.Value{}, fmt.Errorf("cannot convert(bool) %v to %v with value: %v", v.Type(), expectedType, val)
		case reflect.Int:
			return reflect.ValueOf(v.Int() != 0), nil
		case reflect.Uint:
			return reflect.ValueOf(v.Uint() != 0), nil
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(v.Float() != 0), nil
		default:
			return reflect.Value{}, fmt.Errorf("cannot convert(bool) %v to %v with value: %v", v.Type(), expectedType, val)
		}
	}

	// if (expectedType.Kind() == reflect.String || expectedType.Kind() == reflect.Slice || expectedType.Kind() == reflect.Interface) && (v.Kind() == reflect.Slice || v.Kind() == reflect.Map) {
	if expectedType.Kind() == reflect.String && (v.Kind() == reflect.Slice || v.Kind() == reflect.Map) {
		vBytes, err := json.Marshal(val)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("marshal error: %w", err)
		}
		return reflect.ValueOf(string(vBytes)), nil
	}

	if (expectedType.Kind() == reflect.String || expectedType.Kind() == reflect.Slice || expectedType.Kind() == reflect.Interface) && (v.Kind() == reflect.Slice || v.Kind() == reflect.Map) {
		return reflect.ValueOf(val), nil
	}

	return reflect.Value{}, fmt.Errorf("cannot convert %v to %v with value: %v", v.Type(), expectedType, val)
}
