package metadata

import "cli/internal/cli"

func NewMetadataCmd() *cli.Command {
	return &cli.Command{
		Name:        "metadata",
		Aliases:     []string{"meta"},
		Description: "Find and remove file metadata (EXIF, GPS, document properties)",
		Subcommands: []*cli.Command{
			NewShowCmd(),
			NewStripCmd(),
		},
	}
}
