package system

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
	"runtime"
)

func InfoRun(args []string) {
	fmt.Println("=== System Info ===")
	fmt.Printf("OS: %s\n", runtime.GOOS)
	fmt.Printf("Architecture: %s\n", runtime.GOARCH)
	fmt.Printf("CPU Cores: %d\n", runtime.NumCPU())
}

func InfoHelp() {
	fmt.Printf("Usage: %s system info\n", meta.Name)
	fmt.Println()
	fmt.Println("Show the CLI info")
}

func NewInfoCmd() *cli.Command {
	return &cli.Command{
		Name:        "info",
		Description: "Show CLI info",
		Run:         InfoRun,
		Help:        InfoHelp,
	}
}
