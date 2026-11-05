package cli

import (
	"fmt"
	"strings"
)

// PreprocessArgs enforces the CLI's flag style: long flags use two dashes
// (--key) and short flags are a single letter (-k).
func PreprocessArgs(args []string) ([]string, error) {
	for _, arg := range args {
		if arg == "--" {
			break // everything after "--" is positional
		}

		if strings.HasPrefix(arg, "---") {
			return nil, fmt.Errorf("invalid flag (too many dashes): %s", arg)
		}

		if strings.HasPrefix(arg, "--") && len(arg) < 4 {
			return nil, fmt.Errorf("invalid long flag: %s", arg)
		}

		if strings.HasPrefix(arg, "-") &&
			!strings.HasPrefix(arg, "--") &&
			arg != "-" &&
			len(arg) != 2 {
			return nil, fmt.Errorf("invalid short flag: %s (long flags use two dashes)", arg)
		}
	}

	return args, nil
}
