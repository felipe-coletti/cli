package system

import (
	"cli/internal/cli"
	"fmt"
	"os"
	"runtime"
	"text/tabwriter"
	"time"
)

func InfoRun(args []string) {
	hostname, _ := os.Hostname()
	now := time.Now()

	dateFormat := "2006-01-02"
	timeFormat := "15:04:05"
	zoneFormat := "MSTZ07:00"

	currentDate := now.Format(dateFormat)
	currentTime := now.Format(timeFormat)
	currentZone := now.Format(zoneFormat)

	user := os.Getenv("USER")

	if user == "" {
		user = os.Getenv("USERNAME")
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	memUsedMB := float64(m.Alloc) / 1024 / 1024

	workingDir, _ := os.Getwd()

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	fmt.Fprintln(writer, "=== System Info ===")
	fmt.Fprintf(writer, "OS:\t%s\n", runtime.GOOS)
	fmt.Fprintf(writer, "Architecture:\t%s\n", runtime.GOARCH)
	fmt.Fprintf(writer, "CPU Cores:\t%d\n", runtime.NumCPU())
	fmt.Fprintf(writer, "Environment Variables:\t%d\n\n", len(os.Environ()))
	fmt.Fprintln(writer, "=== Host Info ===")
	fmt.Fprintf(writer, "Host Name:\t%s\n", hostname)
	fmt.Fprintf(writer, "User:\t%s\n", user)
	fmt.Fprintf(writer, "Date:\t%s\n", currentDate)
	fmt.Fprintf(writer, "Time:\t%s\n", currentTime)
	fmt.Fprintf(writer, "Time Zone:\t%s\n\n", currentZone)
	fmt.Fprintln(writer, "=== Process Info ===")
	fmt.Fprintf(writer, "PID:\t%d\n", os.Getpid())
	fmt.Fprintf(writer, "Parent PID:\t%d\n", os.Getppid())
	fmt.Fprintf(writer, "Working Dir:\t%s\n", workingDir)
	fmt.Fprintf(writer, "Memory Used:\t%.2f MB\n", memUsedMB)

}

func NewInfoCmd() *cli.Command {
	return &cli.Command{
		Name:        "info",
		Description: "Show CLI info",
		Run:         InfoRun,
	}
}
