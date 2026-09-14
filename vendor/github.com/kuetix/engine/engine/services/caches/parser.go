package caches

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	h "github.com/kuetix/engine/engine/helpers"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

var FunctionCache = domain.FunctionCache{}

func generateModulesJSON(outputPath string) error {
	err := os.MkdirAll(outputPath, 0755)
	if err != nil {
		return err
	}

	type MethodInfo struct {
		Value       string        `json:"value"`
		Label       string        `json:"label"`
		ShortLabel  string        `json:"names"`
		ShortTypes  string        `json:"types"`
		Description string        `json:"description"`
		Method      domain.Method `json:"method"`
	}

	type ModuleInfo struct {
		GoModule    string `json:"go_module"`
		Namespace   string `json:"namespace"`
		Class       string `json:"class"`
		Label       string `json:"label"`
		Description string `json:"description"`
	}

	type Module struct {
		Info    ModuleInfo   `json:"info"`
		Methods []MethodInfo `json:"methods"`
	}

	modules := make(map[string]Module)

	// Build modules structure from FunctionCache
	for service, transitions := range FunctionCache {
		for transition, funcs := range transitions {
			// Create module key: service.transition
			moduleKey := fmt.Sprintf("%s/%s", service, transition)

			// Create module info
			info := ModuleInfo{
				Namespace:   service,
				Class:       transition,
				Label:       helpers.ToCamelCase(transition) + " Module",
				Description: fmt.Sprintf("Handles %s operations for %s service", transition, service),
			}

			var methods []MethodInfo

			goModule, _ := helpers.GetModuleFromGoMod()
			// Sort function names for consistent output
			funcNames := make([]string, 0, len(funcs))
			for name := range funcs {
				funcNames = append(funcNames, name)
			}
			sort.Strings(funcNames)

			for _, funcName := range funcNames {
				mthds := funcs[funcName].Methods
				for _, meta := range mthds {
					// Build method label with parameters
					var params string = ""
					for i, arg := range meta.ArgNames {
						if i > 0 {
							params += ", "
						}
						params += fmt.Sprintf("%s %s", arg, meta.ArgTypes[i])
					}
					shortParams := strings.Join(meta.ArgNames, ", ")
					paramsTypes := strings.Join(meta.ArgTypes, ", ")
					withName := false
					var returns string = ""
					for i, arg := range meta.ReturnNames {
						if i > 0 {
							returns += ", "
						}
						if arg == "" {
							returns += fmt.Sprintf("%s", meta.ReturnTypes[i])
						} else {
							withName = true
							returns += fmt.Sprintf("%s %s", arg, meta.ReturnTypes[i])
						}
					}
					if len(returns) > 1 || withName {
						returns = fmt.Sprintf("(%s)", returns)
					}
					shortReturns := strings.Join(meta.ReturnNames, ", ")
					if len(meta.ReturnNames) > 1 {
						shortReturns = fmt.Sprintf("(%s)", shortReturns)
					}
					returnsTypes := strings.Join(meta.ReturnTypes, ", ")
					if len(meta.ReturnNames) > 1 {
						returnsTypes = fmt.Sprintf("(%s)", returnsTypes)
					}
					label := fmt.Sprintf("%s(%s) %s", meta.Name, params, returns)
					shortNames := fmt.Sprintf("%s(%s) %s", meta.Name, shortParams, shortReturns)
					shortTypes := fmt.Sprintf("%s(%s) %s", meta.Name, paramsTypes, returnsTypes)

					// Build description from types
					description := fmt.Sprintf("Function with %d input(s) and %d output(s)", meta.NumIn, meta.NumOut)

					goModule = meta.GoModule
					meta.Namespace = service
					meta.Class = transition
					methods = append(methods, MethodInfo{
						Value:       meta.Name,
						Label:       label,
						ShortLabel:  shortNames,
						ShortTypes:  shortTypes,
						Description: description,
						Method:      meta,
					})
				}
			}

			info.GoModule = goModule
			modules[moduleKey] = Module{
				Info:    info,
				Methods: methods,
			}
		}
	}

	// Write to JSON file
	modulesFile, err := os.Create(filepath.Join(outputPath, "modules.json"))
	if err != nil {
		return err
	}
	defer func(modulesFile *os.File) {
		err = modulesFile.Close()
		if err != nil {
			logger.Errorf(fmt.Sprintf("Failed to close file: %s", err))
		}
	}(modulesFile)

	encoder := json.NewEncoder(modulesFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(modules); err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("Generated modules.json with %d modules", len(modules)))
	return nil
}

func ParseGoFile(path string, rootPath string, modulesPath string) {
	fSet := token.NewFileSet()
	node, err := parser.ParseFile(fSet, path, nil, 0)
	if err != nil {
		logger.Errorf(fmt.Sprintf("Failed to parse: %s", err))
		return
	}

	service, transition := extractServiceAndTransition(path, modulesPath)
	goModuleNamespace, err := helpers.GetModuleFromGoMod()
	if err != nil {
		logger.Errorf(fmt.Sprintf("Failed to get module from go.mod: %s", err))
		return
	}
	methods := []domain.Method{}
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name == nil {
			continue
		}
		if funcDecl.Name.IsExported() == false {
			continue
		}
		// Skip functions without receivers (not methods)
		if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}
		// Skip constructor functions that return ServiceTransitions interface
		if funcDecl.Type.Results != nil {
			var skip = false
			for _, result := range funcDecl.Type.Results.List {
				typeStr := helpers.ExprToString(result.Type)
				if typeStr == "interfaces.ServiceTransitions" {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}

		var inputs, argNames, outputs, returnNames []string
		var numIn, numOut int

		// Inputs
		if funcDecl.Type.Params != nil {
			for _, field := range funcDecl.Type.Params.List {
				typeStr := helpers.ExprToString(field.Type)
				if len(field.Names) > 0 {
					for _, name := range field.Names {
						inputs = append(inputs, typeStr)
						argNames = append(argNames, name.Name)
						numIn++
					}
				} else {
					inputs = append(inputs, typeStr)
					argNames = append(argNames, fmt.Sprintf("in%d", len(argNames)))
					numIn++
				}
			}
		}

		// Outputs
		if funcDecl.Type.Results != nil {
			for _, field := range funcDecl.Type.Results.List {
				typeStr := helpers.ExprToString(field.Type)
				if len(field.Names) > 0 {
					for _, name := range field.Names {
						outputs = append(outputs, typeStr)
						returnNames = append(returnNames, name.Name)
						numOut++
					}
				} else {
					outputs = append(outputs, typeStr)
					returnNames = append(returnNames, fmt.Sprintf("out%d", len(returnNames)))
					numOut++
				}
			}
		}

		recvType := helpers.ExprToString(funcDecl.Recv.List[0].Type)
		methods = append(methods, domain.Method{
			GoModule:     goModuleNamespace,
			ModulePath:   modulesPath,
			FilePath:     path,
			Namespace:    transition,
			Class:        service,
			Name:         funcDecl.Name.Name,
			ReceiverType: recvType,
			NumIn:        numIn,
			NumOut:       numOut,
			ArgTypes:     inputs,
			ReturnTypes:  outputs,
			ArgNames:     argNames,
			ReturnNames:  returnNames,
			FuncDecl:     funcDecl,
		})
	}

	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name == nil {
			continue
		}

		var inputs, argNames, outputs, returnNames []string

		// Inputs
		if funcDecl.Type.Params != nil {
			for _, field := range funcDecl.Type.Params.List {
				typeStr := helpers.ExprToString(field.Type)
				if len(field.Names) > 0 {
					for _, name := range field.Names {
						inputs = append(inputs, typeStr)
						argNames = append(argNames, name.Name)
					}
				} else {
					// anonymous parameter
					inputs = append(inputs, typeStr)
					argNames = append(argNames, fmt.Sprintf("in%d", len(argNames)))
				}
			}
		}

		// Outputs
		if funcDecl.Type.Results != nil {
			for _, field := range funcDecl.Type.Results.List {
				typeStr := helpers.ExprToString(field.Type)
				if len(field.Names) > 0 {
					for _, name := range field.Names {
						outputs = append(outputs, typeStr)
						returnNames = append(returnNames, name.Name)
					}
				} else {
					outputs = append(outputs, typeStr)
					returnNames = append(returnNames, fmt.Sprintf("out%d", len(returnNames)))
				}
			}
		}

		if len(outputs) < 1 {
			continue
		}

		var found = false
		for _, outType := range outputs {
			if outType == "interfaces.ServiceTransitions" {
				found = true
				break
			}
		}

		if !found {
			continue
		}

		service, transition = extractServiceAndTransition(path, modulesPath)

		if FunctionCache[service] == nil {
			FunctionCache[service] = map[string]map[string]domain.FunctionMetadata{}
		}
		if FunctionCache[service][transition] == nil {
			FunctionCache[service][transition] = map[string]domain.FunctionMetadata{}
		}

		FunctionCache[service][transition][funcDecl.Name.Name] = domain.FunctionMetadata{
			GoModule:    goModuleNamespace,
			ModulePath:  modulesPath,
			FilePath:    path,
			Namespace:   transition,
			Class:       service,
			Name:        funcDecl.Name.Name,
			NumIn:       len(inputs),
			NumOut:      len(outputs),
			ArgTypes:    inputs,
			ReturnTypes: outputs,
			ArgNames:    argNames,
			ReturnNames: returnNames,
			Methods:     methods,
		}

		// Add each method to the cache
		for _, method := range methods {
			FunctionCache[service][transition][method.Name] = domain.FunctionMetadata{
				GoModule:    goModuleNamespace,
				ModulePath:  modulesPath,
				FilePath:    path,
				Namespace:   transition,
				Class:       service,
				Name:        method.Name,
				NumIn:       method.NumIn,
				NumOut:      method.NumOut,
				ArgTypes:    method.ArgTypes,
				ReturnTypes: method.ReturnTypes,
				ArgNames:    method.ArgNames,
				ReturnNames: method.ReturnNames,
				Methods:     []domain.Method{method},
			}
			logger.Info(fmt.Sprintf("Method: %s | Args: %v | Returns: %v", method.Name, method.ArgNames, method.ReturnNames))
		}

		logger.Info(fmt.Sprintf("Func: %s | Args: %v | Returns: %v", funcDecl.Name.Name, argNames, returnNames))
	}

	// At the comment location in ParseGoFile:
	if len(FunctionCache) > 0 {
		if err = generateModulesJSON(filepath.Join(rootPath, modulesPath)); err != nil {
			logger.Errorf(fmt.Sprintf("Failed to generate modules.json: %s", err))
		}
	}
}

func GenerateMetaFiles(moduleName, modulesPath string, cachePath string) error {
	outputPath := filepath.Join(modulesPath, cachePath)
	di := map[string]map[string]map[string]domain.FunctionMetadata{}

	err := os.MkdirAll(filepath.Dir(outputPath), 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(outputPath, "meta.go"))
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		if cerr := f.Close(); cerr != nil {
			logger.Errorf(fmt.Sprintf("Failed to close file: %s", cerr))
		}
	}(f)

	helpers.Fprintf(f, `// Code generated by kueinit. DO NOT EDIT.
package modules

import (
	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/domain/interfaces"
)

func init() {
	boot.AddMetaFunctionCache(map[string]map[string]map[string]interfaces.FunctionMetadata{
`)
	for service, transitions := range FunctionCache {
		_, err = fmt.Fprintf(f, "\t\t%q: {\n", service)
		if err != nil {
			return err
		}
		// strings.Join([]string{service, transition}, "/")
		for transition, funcs := range transitions {
			_, err = fmt.Fprintf(f, "\t\t\t%q: {\n", transition)
			if err != nil {
				return err
			}
			for funcName, meta := range funcs {
				if len(meta.ReturnTypes) == 1 && meta.ReturnTypes[0] == "interfaces.ServiceTransitions" {
					if di[service] == nil {
						di[service] = map[string]map[string]domain.FunctionMetadata{}
					}

					if di[service][transition] == nil {
						di[service][transition] = map[string]domain.FunctionMetadata{}
					}

					di[service][transition][funcName] = meta
				} else {
					_, err = fmt.Fprintf(f,
						"\t\t\t\t%q: {\n"+
							"\t\t\t\t\tGoModule: %q,\n"+
							"\t\t\t\t\tModulePath: %q,\n"+
							"\t\t\t\t\tFilePath: %q,\n"+
							"\t\t\t\t\tNamespace: %q,\n"+
							"\t\t\t\t\tClass: %q,\n"+
							"\t\t\t\t\tName: %q,\n"+
							"\t\t\t\t\tNumIn: %d,\n"+
							"\t\t\t\t\tNumOut: %d,\n"+
							"\t\t\t\t\tArgTypes: []string{%s},\n"+
							"\t\t\t\t\tReturnTypes: []string{%s},\n"+
							"\t\t\t\t\tArgNames: []string{%s},\n"+
							"\t\t\t\t\tReturnNames: []string{%s},\n"+
							"\t\t\t\t},\n",
						funcName,
						meta.GoModule,
						meta.ModulePath,
						meta.FilePath,
						meta.Namespace,
						meta.Class,
						meta.Name,
						meta.NumIn,
						meta.NumOut,
						quoteSlice(meta.ArgTypes),
						quoteSlice(meta.ReturnTypes),
						quoteSlice(meta.ArgNames),
						quoteSlice(meta.ReturnNames),
					)
					if err != nil {
						return err
					}
				}
			}
			helpers.Fprintln(f, "\t\t\t},")
		}
		helpers.Fprintln(f, "\t\t},")
	}

	helpers.Fprintln(f, "\t})\n}\n")

	diFile, err := os.Create(outputPath + "/di.go")
	if err != nil {
		return err
	}
	defer func(diFile *os.File) {
		err := diFile.Close()
		if err != nil {
			logger.Errorf(fmt.Sprintf("Failed to close file: %s", err))
		}
	}(diFile)

	// Collect all services for import
	imported := map[string]bool{}
	importPath := h.ModulesImportPath(modulesPath)

	moduleNameCut := strings.TrimPrefix(moduleName, "github.com/")
	importNames := strings.Split(importPath, moduleNameCut)
	var importName = filepath.Clean(moduleNameCut)
	if len(importNames) > 1 {
		importName = filepath.Clean(strings.Join(importNames[1:], "/"))
	} else {
		importName = filepath.Clean(strings.Join(importNames, "/"))
	}
	importPath = strings.TrimPrefix(importName, "/")

	imports := map[string]string{}
	for service := range di {
		serviceCamel := helpers.ToCamelCase(helpers.SanitizeServiceName(service))
		if !imported[service] {
			path := filepath.Join(helpers.CleanSlashes(moduleName, importPath, service, "transitions")...)
			imported[service] = true
			imports[serviceCamel] = path
		}
	}
	if len(imports) == 0 {

		helpers.Fprintf(diFile, `// Code generated by kueinit. DO NOT EDIT.
package modules

import (
	di "github.com/kuetix/container"
`)
	} else {

		helpers.Fprintf(diFile, `// Code generated by kueinit. DO NOT EDIT.
package modules

import (
	di "github.com/kuetix/container"
	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/engine/engine/workflow"
`)
	}

	for serviceCamel, path := range imports {
		helpers.Fprintf(diFile, "\ttransitions%s \"%s\"\n", serviceCamel, path)
	}
	helpers.Fprintf(diFile, `)

func init() {
	di.Boot()
`)

	diNames := make([]string, 0, len(di))
	for name := range di {
		diNames = append(diNames, name)
	}
	sort.Strings(diNames)
	for _, service := range diNames {
		serviceCamel := helpers.ToCamelCase(helpers.SanitizeServiceName(service))
		transitions := di[service]
		helpers.Fprintf(diFile, "\tdi.DependencyInjection[%q] = func(name string) {\n", service)
		trNames := make([]string, 0, len(transitions))
		for name := range transitions {
			trNames = append(trNames, name)
		}
		sort.Strings(trNames)
		for _, transition := range trNames {
			fnNames := make([]string, 0, len(transitions[transition]))
			for name := range transitions[transition] {
				fnNames = append(fnNames, name)
			}
			sort.Strings(fnNames)
			for _, fnName := range fnNames {
				helpers.Fprintf(diFile, "\t\tdi.ToResolve(defines.TransitionPrefix+%q+%q+%q, func() interface{} { return workflow.ServiceTransitionMapping{ ServiceName: name, Name: %q, Impl: transitions%s.%s() }})\n", service, "/", transition, transition, serviceCamel, fnName)
			}
		}
		helpers.Fprintln(diFile, "\t}")
	}

	helpers.Fprintln(diFile, "}")

	return err
}

func quoteSlice(slice []string) string {
	if len(slice) == 0 {
		return ""
	}
	quoted := make([]string, len(slice))
	for i, s := range slice {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(quoted, ", ")
}

func extractServiceAndTransition(path string, modulesPath string) (string, string) {
	clean := filepath.Clean(modulesPath)
	packagesPath := strings.TrimPrefix(path, clean+"/")
	parts := strings.Split(filepath.ToSlash(packagesPath), "/")

	// Find "transitions" and work backward
	for i := len(parts) - 1; i >= 1; i-- {
		if parts[i] == "transitions" {
			serviceParts := parts[0:i]                 // everything between "modules" and "transitions"
			service := strings.Join(serviceParts, "/") // skip "modules"
			transition := strings.TrimSuffix(filepath.Base(path), ".go")
			return service, transition
		}
	}
	return "unknown", "unknown"
}
