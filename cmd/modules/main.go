package main

import (
	"fmt"
	"time"

	"github.com/kuetix/container"
	"github.com/kuetix/engine/boot"
	"github.com/kuetix/kue/modules"
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
	boot.DependencyInjection()

	fmt.Printf("Modules package initialized. Version: %s, BuildTime: %s\n", Version, BuildTime)
	for name, funcs := range container.DependencyInjection {
		fmt.Printf("DependencyInjectionContainer - %s: %v\n", name, funcs)
	}
	for name, funcs := range container.FactoryContainer {
		fmt.Printf("FactoryContainer - %s: %v\n", name, funcs)
	}
	for fc1name, transitions := range boot.MetaFunctionCache {
		for fc2name, services := range transitions {
			for _, meta := range services {
				inputs := ""
				for i, argName := range meta.ArgNames {
					if i > 0 {
						inputs += ", "
					}
					inputs += fmt.Sprintf("%s: %s", argName, meta.ArgTypes[i])
				}
				returns := ""
				for i, argName := range meta.ReturnNames {
					if i > 0 {
						returns += ", "
					}
					returns += fmt.Sprintf("%s: %s", argName, meta.ReturnTypes[i])
				}
				fmt.Printf("%s/%s.%s(%s) (%s) \n", fc1name, fc2name, meta.Name, inputs, returns)
			}
		}
	}
}
