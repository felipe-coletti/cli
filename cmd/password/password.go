package password

import (
	"cli/internal/cli"
	"fmt"
)

func PasswordHelp() {
	fmt.Println("Usage: cli password <subcommand>")
	fmt.Println()
	fmt.Println("Available subcommands:")
	fmt.Printf("  %s    %s\n", NewGenerateCmd().Name, NewGenerateCmd().Description)
}

func NewPasswordCmd() *cli.Command {
	return &cli.Command{
		Name:        "password",
		Description: "Password related commands",
		Help:        PasswordHelp,
		Subcommands: []*cli.Command{
			NewGenerateCmd(),
		},
	}
}
