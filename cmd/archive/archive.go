package archive

import "cli/internal/cli"

func NewArchiveCmd() *cli.Command {
	return &cli.Command{
		Name:        "archive",
		Aliases:     []string{"arch"},
		Description: "Archive related commands",
		Subcommands: []*cli.Command{
			NewPackCmd(),
			NewExtractCmd(),
		},
	}
}
