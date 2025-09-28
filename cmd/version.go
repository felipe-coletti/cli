package cmd

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
)

func VersionRun(args []string) {
	fmt.Println(meta.Version)
}

func NewVersionCmd() *cli.Command {
	return &cli.Command{
		Name:        "version",
		Description: "Show CLI version",
		Run:         VersionRun,
	}
}
