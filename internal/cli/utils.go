package cli

import (
	"fmt"
	"os"
	"strings"
)

func PreprocessArgs(args []string) []string {
	validArgs := []string{}

	for _, arg := range args {
		if arg == "-" || arg == "--" {
			continue
		} else if strings.HasPrefix(arg, "---") {
			fmt.Fprintf(os.Stderr, "invalid flag (too many dashes): %s\n", arg)
			os.Exit(1)
		} else if strings.HasPrefix(arg, "--") && len(arg) < 4 {
			fmt.Fprintf(os.Stderr, "invalid long flag: %s\n", arg)
			os.Exit(1)
		} else if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) != 2 {
			fmt.Fprintf(os.Stderr, "invalid short flag: %s\n", arg)
			os.Exit(1)
		}

		validArgs = append(validArgs, arg)
	}

	return validArgs
}
