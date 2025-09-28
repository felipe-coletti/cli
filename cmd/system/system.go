package system

import "cli/internal/cli"

func NewSystemCmd() *cli.Command {
	return &cli.Command{
		Name:        "system",
		Aliases:     []string{"sys"},
		Description: "System related commands",
		Subcommands: []*cli.Command{
			NewDateCmd(),
			NewInfoCmd(),
		},
	}
}
