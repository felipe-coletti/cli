package system

import (
	"cli/internal/cli"
	"fmt"
)

func SystemHelp() {
	fmt.Println("Usage: cli system <subcommand>")
	fmt.Println()
	fmt.Println("Available subcommands:")
	fmt.Printf("  %s    %s\n", NewDateCmd().Name, NewDateCmd().Description)
	fmt.Printf("  %s    %s\n", NewInfoCmd().Name, NewInfoCmd().Description)
}

func NewSystemCmd() *cli.Command {
	return &cli.Command{
		Name:        "system",
		Aliases:     []string{"sys"},
		Description: "System related commands",
		Help:        SystemHelp,
		Subcommands: []*cli.Command{
			NewDateCmd(),
			NewInfoCmd(),
		},
	}
}
