package helpers

// ArgsReorg reorganizes the arguments to be in the format of flags and non-flags
//
//goland:noinspection GoUnusedExportedFunction
func ArgsReorg(args ...string) []string {
	inputs := args
	var arguments []string
	for _, arg := range inputs {
		if arg[0] == '-' {
			// Remaining args are not flags
			arguments = append(arguments, arg)
			break
		}
	}
	for _, arg := range inputs {
		if arg[0] != '-' {
			// Remaining args are not flags
			arguments = append(arguments, arg)
			break
		}
	}

	return arguments
}
