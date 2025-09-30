package password

import (
	"crypto/rand"
	"fmt"
	"math/big"
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

func Generate(length int, uppercase, lowercase, numbers, symbols bool) string {
	const (
		upperSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lowerSet = "abcdefghijklmnopqrstuvwxyz"
		numSet   = "0123456789"
		symSet   = "!@#$%^&*()-_=+"
	)

	charset := ""

	if uppercase {
		charset += upperSet
	}
	if lowercase {
		charset += lowerSet
	}
	if numbers {
		charset += numSet
	}
	if symbols {
		charset += symSet
	}

	if len(charset) == 0 {
		charset += "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()-_=+"
	}

	result := make([]byte, length)

	for i := range length {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))

		result[i] = charset[num.Int64()]
	}

	return string(result)
}
