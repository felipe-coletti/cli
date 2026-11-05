package cmd

import (
	"cli/cmd/archive"
	"cli/cmd/crypto"
	"cli/cmd/metadata"
	"cli/cmd/password"
	"cli/cmd/system"
	"cli/internal/cli"
	"errors"
	"fmt"
	"os"
)

func Commands() []*cli.Command {
	return []*cli.Command{
		NewVersionCmd(),
		archive.NewArchiveCmd(),
		crypto.NewCryptoCmd(),
		metadata.NewMetadataCmd(),
		password.NewPasswordCmd(),
		system.NewSystemCmd(),
	}
}

// Main runs the CLI and returns the process exit code:
// 0 success, 1 runtime error, 2 usage error.
func Main(args []string) int {
	err := cli.Execute(Commands(), args)
	if err == nil {
		return 0
	}

	var ue *cli.UsageError
	if errors.As(err, &ue) {
		cli.PrintUsageError(ue)
		return 2
	}

	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	return 1
}
