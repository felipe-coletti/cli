package system

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
	"time"
)

func DateRun(args []string) {
	format := "2006-01-02 15:04:05 MSTZ07:00"

	if len(args) > 0 {
		format = args[0]
	}

	now := time.Now()

	fmt.Println(now.Format(format))
}

func DateHelp() {
	fmt.Printf("Usage: %s date [format]\n", meta.Name)
	fmt.Println()
	fmt.Println("Show the current date and time")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s date    # yyyy-mm-dd hh:mm:ss\n", meta.Name)
	fmt.Printf("  %s time    # yyyy-mm-dd hh:mm:ss\n", meta.Name)
	fmt.Printf("  %s date yyyy-mm-dd\n", meta.Name)
	fmt.Printf("  %s date hh:mm:ss\n", meta.Name)
}

func NewDateCmd() *cli.Command {
	return &cli.Command{
		Name:        "date",
		Aliases:     []string{"time"},
		Description: "Show the current date and time",
		Run:         DateRun,
		Help:        DateHelp,
	}
}
