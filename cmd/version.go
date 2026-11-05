package cmd

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// buildInfo describes the binary using the VCS data the Go toolchain embeds
// automatically (commit, date, dirty tree), no -ldflags required.
func buildInfo() string {
	parts := []string{}

	if bi, ok := debug.ReadBuildInfo(); ok {
		settings := map[string]string{}
		for _, s := range bi.Settings {
			settings[s.Key] = s.Value
		}

		if rev := settings["vcs.revision"]; rev != "" {
			commit := rev[:min(len(rev), 7)]
			if settings["vcs.modified"] == "true" {
				commit += "-dirty"
			}
			parts = append(parts, "commit "+commit)
		}
		if t := settings["vcs.time"]; len(t) >= 10 {
			parts = append(parts, t[:10])
		}
	}

	parts = append(parts, runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH)
	return strings.Join(parts, ", ")
}

func NewVersionCmd() *cli.Command {
	var short bool

	return &cli.Command{
		Name:        "version",
		Description: "Show CLI version",
		Flags: func(fs *cli.FlagSet) {
			fs.BoolVar(&short, "short", "s", false, "Print only the version number")
		},
		Run: func(args []string) error {
			if short {
				fmt.Println(meta.Version)
				return nil
			}
			fmt.Printf("%s %s (%s)\n", meta.Name, meta.Version, buildInfo())
			return nil
		},
	}
}
