package system

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
	"os"
	"text/tabwriter"
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
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	fmt.Printf("Usage: %s system date [format]\n\n", meta.Name)
	fmt.Println("Show the current date and time")
	fmt.Println("\nExamples:")
	fmt.Fprintf(writer, "  %s date\t# yyyy-mm-dd hh:mm:ss\n", meta.Name)
	fmt.Fprintf(writer, "  %s time\t# yyyy-mm-dd hh:mm:ss\n", meta.Name)
	fmt.Fprintf(writer, "  %s date 2006-01-02\n", meta.Name)
	fmt.Fprintf(writer, "  %s date 15:04:05\n", meta.Name)
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
