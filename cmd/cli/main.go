package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kuetix/engine"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/kue"
	"github.com/kuetix/kue/modules"
	"github.com/kuetix/logger"
)

var Version string
var BuildTime string

func main() {
	if Version == "" {
		Version = "dev"
	}

	if BuildTime == "" {
		BuildTime = time.Now().Format(time.RFC3339)
	}

	modules.Enable()

	// std-core's services/common/log package flips Info logging on in an
	// init(), so every kue run otherwise prints engine chatter (restart
	// policy, "no resolvers defined", ...) to stdout. Keep the console
	// quiet unless the caller actually asked for -v/-vv; std-cli re-enables
	// the level itself when the verbose/debug flag is set.
	if !hasVerboseFlag(os.Args) {
		logger.DisableInfo()
		logger.DisableWarn()
	}

	// Run the API server startup workflow using engine.RunWorkflow
	options := &domain.Options{
		EngineName:    "cli-pkg",
		ConfigName:    "engine",
		Verbose:       false,
		Quiet:         false,
		Amount:        1,
		Retry:         1,
		RetryDelay:    0,
		RestartPolicy: "",
		Workflow:      "@cli/startup",
		Version:       Version,
		BuildTime:     BuildTime,
		LogPath:       "stdout",
		Config:        &domain.Config{},
		Args: []string{
			"Version: " + Version,
			"BuildTime: " + BuildTime,
		},
		Context:         map[string]interface{}{},
		EmbedFS:         &kue.WorkflowsFS,
		EmbedFSRootPath: kue.WorkflowsFSPath,
	}

	response := engine.RunWorkflow("production", options)

	var errs = make(map[string]interface{})
	for _, res := range response {
		if res.Response != nil {
			//fmt.Printf("Result: %v\n", res.Response)
			if r, ok := res.Response.(map[string]interface{}); ok {
				var requestedCommand = ""
				if txt, ok := r["requestedCommand"]; ok {
					requestedCommand = txt.(string)
					fmt.Printf("Command %s", JsonResponse(txt))
				}
				if txt, ok := r["message"]; ok {
					fmt.Printf("Message %s", JsonResponse(txt))
				}
				if txt, ok := r["availableCommands"]; ok {
					similar := []string{}
					if requestedCommand != "" {
						p := strings.Split(requestedCommand, ".")
						rc := requestedCommand
						if len(p) > 0 {
							rc = p[0]
						}
						for _, i := range txt.([]string) {
							if strings.Contains(i, rc) {
								similar = append(similar, i)
							}
						}
					} else {
						similar = txt.([]string)
					}
					fmt.Printf("AvailableCommands:\n%s", JsonResponse(similar))
				}
			} else {
				if s, ok := res.Response.(string); ok {
					fmt.Printf("%s", s)
				} else {
					fmt.Printf("%s", JsonResponse(res.Response))
				}
			}
		}
		if res.Error != nil && res.Error.HasIssues() {
			printed := false
			for _, i := range res.Error.Errors() {
				if len(i.Errors) > 0 {
					err := i.Errors[0]
					s := err.Error()
					if _, ok := errs[s]; !ok {
						errs[s] = i
						if s != "" {
							fmt.Printf("Error: %s\n", s)
							printed = true
						}
					} else if s != "" {
						printed = true
					}
				}
			}
			// Only fail the process when there is a real error to report — the
			// engine can attach an empty issues bag to an otherwise-successful
			// run.
			if printed {
				os.Exit(1)
			}
		}
	}
}

// hasVerboseFlag reports whether the invocation asked for verbose or debug
// logging, in any of the spellings the CLI accepts.
func hasVerboseFlag(args []string) bool {
	for _, a := range args {
		switch a {
		case "-v", "--v", "-verbose", "--verbose", "-vv", "--vv", "-debug", "--debug":
			return true
		}
	}
	return false
}

func JsonResponse(response any) string {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false) // <-- disables \u003e escaping
	enc.SetIndent("", "  ")

	if err := enc.Encode(response); err != nil {
		logger.Errorf("enc.Encode error: %s", err)
	}

	return buf.String()
}
