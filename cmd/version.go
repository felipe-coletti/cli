package cmd

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
)

func VersionRun(args []string) {
	fmt.Println(meta.Version)
}

func VersionHelp() {
	fmt.Printf("Usage: %s version\n", meta.Name)
	fmt.Println()
	fmt.Println("Show the CLI version")
}

func NewVersionCmd() *cli.Command {
	return &cli.Command{
		Name:        "version",
		Description: "Show CLI version",
		Run:         VersionRun,
		Help:        VersionHelp,
	}
}
