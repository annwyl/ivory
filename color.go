package ivory

import "os"

// only colorize when we're actually writing to a terminal, otherwise piped. prob obsolete soon
var useColor = isTerminal()

func isTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func green(s string) string {
	if !useColor {
		return s
	}
	return "\033[32m" + s + "\033[0m"
}

func red(s string) string {
	if !useColor {
		return s
	}
	return "\033[31m" + s + "\033[0m"
}
