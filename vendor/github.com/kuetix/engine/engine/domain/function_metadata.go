package domain

import "go/ast"

type Method struct {
	GoModule     string        `json:"go_module" mapstructure:"go_module"`
	FilePath     string        `json:"file_path" mapstructure:"file_path"`
	ModulePath   string        `json:"module_path" mapstructure:"module_path"`
	Namespace    string        `json:"namespace" mapstructure:"namespace"`
	Class        string        `json:"class" mapstructure:"class"`
	Name         string        `json:"name" mapstructure:"name"`
	ReceiverType string        `json:"receiver_type" mapstructure:"receiver_type"`
	NumIn        int           `json:"num_in" mapstructure:"num_in"`
	NumOut       int           `json:"num_out" mapstructure:"num_out"`
	ArgTypes     []string      `json:"arg_types" mapstructure:"arg_types"`
	ReturnTypes  []string      `json:"return_types" mapstructure:"return_types"`
	ArgNames     []string      `json:"arg_names" mapstructure:"arg_names"`
	ReturnNames  []string      `json:"return_names" mapstructure:"return_names"`
	FuncDecl     *ast.FuncDecl `json:"-" mapstructure:"-"`
}

type FunctionMetadata struct {
	GoModule    string   `json:"go_module" mapstructure:"go_module"`
	ModulePath  string   `json:"module_path" mapstructure:"module_path"`
	FilePath    string   `json:"file_path" mapstructure:"file_path"`
	Namespace   string   `json:"namespace" mapstructure:"namespace"`
	Class       string   `json:"class" mapstructure:"class"`
	Name        string   `json:"name" mapstructure:"name"`
	NumIn       int      `json:"num_in" mapstructure:"num_in"`
	NumOut      int      `json:"num_out" mapstructure:"num_out"`
	ArgTypes    []string `json:"arg_types" mapstructure:"arg_types"`
	ReturnTypes []string `json:"return_types" mapstructure:"return_types"`
	ArgNames    []string `json:"arg_names" mapstructure:"arg_names"`
	ReturnNames []string `json:"return_names" mapstructure:"return_names"`
	Methods     []Method `json:"methods" mapstructure:"methods"`
}
