package boot

import (
	di "github.com/kuetix/container"
)

func DependencyInjection() *di.FactoryMap {
	EnableBoot()

	di.DependencyInjectionBoot()

	return &di.FactoryContainer
}
