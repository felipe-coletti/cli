package password

import "cli/internal/cli"

func NewPasswordCmd() *cli.Command {
	return &cli.Command{
		Name:        "password",
		Description: "Password related commands",
		Subcommands: []*cli.Command{
			NewGenerateCmd(),
		},
	}
}
