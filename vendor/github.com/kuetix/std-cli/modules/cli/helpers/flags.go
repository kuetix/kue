package helpers

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/workflow"
)

func FlagInt(fs *flag.FlagSet, long, short, usage string, value int) []*int {
	l := fs.Int(long, value, usage)
	s := fs.Int(short, value, usage+" (shorthand)")
	return []*int{l, s}
}

func FlagBool(fs *flag.FlagSet, long, short, usage string, value bool) []*bool {
	l := fs.Bool(long, value, usage)
	s := fs.Bool(short, value, usage+" (shorthand)")
	return []*bool{l, s}
}

func FlagString(fs *flag.FlagSet, long, short, usage, value string) []*string {
	l := fs.String(long, value, usage)
	s := fs.String(short, value, usage+" (shorthand)")
	return []*string{l, s}
}

func IntArg(defaultValue int, flags ...*int) func() *int {
	return func() *int {
		for _, f := range flags {
			if f != nil && *f != defaultValue {
				return f
			}
		}
		return &defaultValue
	}
}

func BoolArg(defaultValue bool, flags ...*bool) func() *bool {
	return func() *bool {
		for _, f := range flags {
			if *f != defaultValue {
				return f
			}

		}
		return &defaultValue
	}
}

func StringArg(defaultValue string, flags ...*string) func() *string {
	return func() *string {
		for _, f := range flags {
			if strings.TrimSpace(*f) != "" && strings.TrimSpace(*f) != defaultValue {
				*f = strings.TrimSpace(*f)
				return f
			}
		}
		return &defaultValue
	}
}

// GetArgs splits os.Args into the command/subcommand, positional args, and
// raw option tokens. It has no access to a resolved command's flag schema
// (the command isn't even identified yet), so — beyond the cases boolFlags
// covers — it falls back to the historical assumption that a "-"-prefixed
// token is followed by a separate value token. boolFlags names every flag
// (long and short spellings) declared with "type": "bool" anywhere in
// commands.wsl, collected up front by the caller; those tokens (and any
// token already carrying its value via "--flag=value") are known to be
// self-contained, so the token right after them is never swallowed as a
// bogus value. Without this, "-v" (a global bool flag) ahead of the command
// consumed the command word itself as -v's "value", and a per-command bool
// flag like "--public" ahead of its workflow name consumed the name the
// same way.
func GetArgs(boolFlags map[string]bool) (mainCommand string, command string, args []string, options []string) {
	args = []string{}
	options = []string{}
	nextIsOptionValue := false
	foundCommand := false
	foundSubcommand := false
	mainCommand = ""

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		isFlag := strings.HasPrefix(arg, "-")

		if isFlag {
			nextIsOptionValue = false
		}

		if nextIsOptionValue {
			nextIsOptionValue = false
			options = append(options, arg)
			continue
		}

		// Found the command (e.g., "add")
		if !foundCommand && !isFlag {
			foundCommand = true
			command = arg
			mainCommand = arg
			continue
		}

		// Found the subcommand (e.g., "module")
		if foundCommand && !foundSubcommand && !isFlag {
			foundSubcommand = true
			command = command + "." + arg
			continue
		}

		if foundSubcommand && !isFlag {
			args = append(args, arg)
		} else {
			options = append(options, arg)
			if !isSelfContainedFlag(arg, boolFlags) {
				nextIsOptionValue = true
			}
		}
	}

	return mainCommand, command, args, options
}

// isSelfContainedFlag reports whether arg is a flag token that never takes a
// separately-tokenized value: it already embeds one ("--flag=value") or its
// bare long/short name (dashes stripped) is a known boolean flag.
func isSelfContainedFlag(arg string, boolFlags map[string]bool) bool {
	if strings.Contains(arg, "=") {
		return true
	}
	return boolFlags[strings.TrimLeft(arg, "-")]
}

func GetFlag(option any) (value any) {
	switch option.(type) {
	case func() *string:
		value = option.(func() *string)()
		if value.(*string) == nil {
			value = ""
		} else {
			value = *value.(*string)
		}
	case func() *int:
		value = option.(func() *int)()
		if value.(*int) == nil {
			value = 0
		} else {
			value = *value.(*int)
		}
	case func() *bool:
		value = option.(func() *bool)()
		if value.(*bool) != nil && *value.(*bool) == true {
			value = true
		} else {
			value = false
		}
	default:
		value = nil
	}

	return
}

func GetFlags(flags map[string]interface{}) (r map[string]interface{}) {
	r = make(map[string]interface{})
	for name, option := range flags {
		r[name] = GetFlag(option)
	}

	return
}

func GetUsage(app domain.Application, usage string, flagSet *flag.FlagSet, rootPath string) string {
	var helpText string = ""
	if usage != "" {
		if strings.HasPrefix(usage, "file://") {
			usage = strings.Trim(usage, "\n\n\t ")
			file := strings.TrimPrefix(usage, "file://")
			filePath := filepath.Join(rootPath, file)
			content := ReadFileAsString(app, filePath)
			if !strings.HasPrefix(content, "err:") {
				usage = content
			}
		}

		if flagSet != nil {
			var buf bytes.Buffer
			flagSet.SetOutput(&buf)
			flagSet.Usage()
			helpText = buf.String() + "\n"
		}

		helpText = usage + "\n"

		return helpText
	}

	return helpText
}

// RenderHelp resolves a command's help text for a `--help` request. It reads
// config["usage"] (a `file://` reference into the workflows FS, or literal
// text) and returns the resolved content. It tolerates a nil session or a
// session without an engine (unit tests call transitions directly), and a
// missing config["flagSet"].
func RenderHelp(ctx *workflow.WorkerSessionContext, config map[string]interface{}, flags map[string]interface{}) string {
	var app domain.Application
	if ctx != nil && ctx.Engine != nil {
		app = ctx.Engine.GetApplication()
	}
	usage, _ := config["usage"].(string)
	var fs *flag.FlagSet
	if f, ok := config["flagSet"].(*flag.FlagSet); ok {
		fs = f
	}
	root, _ := GetFlags(flags)["workflows"].(string)
	return GetUsage(app, usage, fs, root)
}

//goland:noinspection GoUnusedExportedFunction
func ReadFileAsString(app domain.Application, path string) string {
	var candidates = []string{
		fmt.Sprintf("%s/%s", app.EmbedFSRootPath, path),
		fmt.Sprintf("%s", path),
	}
	for _, candidate := range candidates {
		content, err := app.EmbedFS.ReadFile(candidate)
		if err == nil {
			return string(content)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "err:" + err.Error()
	}
	return string(content)
}
