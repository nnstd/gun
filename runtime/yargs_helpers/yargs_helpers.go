package yargs_helpers

// HideBin removes the first two elements from os.Args (the binary path
// and the script path), matching the behavior of yargs/helpers hideBin.
func HideBin(args []string) []string {
	if len(args) > 2 {
		return args[2:]
	}
	if len(args) > 1 {
		return args[1:]
	}
	return []string{}
}
