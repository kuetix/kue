package container

var initOnce = false
var DependencyInjection map[string]func(string)

func Boot() {
	if initOnce {
		return
	}
	DependencyInjection = make(map[string]func(string))

	initOnce = true
}

// Booted reports whether Boot() has already been executed.
func Booted() bool {
	return initOnce
}

// ResetBootstrap resets bootstrap state (intended for tests).
func ResetBootstrap() {
	initOnce = false
	DependencyInjection = nil
}

func DependencyInjectionBoot() {
	for name, boot := range DependencyInjection {
		boot(name)
	}
}
