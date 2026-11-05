package system

import (
	"bufio"
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// memTotal returns total RAM in bytes (Linux only, 0 if unknown).
func memTotal() uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, _ := strconv.ParseUint(fields[1], 10, 64)
			return kb * 1024
		}
	}
	return 0
}

// uptime returns the system uptime (Linux only, 0 if unknown).
func uptime() time.Duration {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}

func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return os.Getenv("USERNAME")
}

// shortInfo prints everything on one line, e.g.
// felipe@node1  linux/amd64  go1.24.7  8c/16g  up 3d4h  id 3f9c0e1d2a7b4c55
func shortInfo() {
	host, _ := os.Hostname()
	parts := []string{
		currentUser() + "@" + host,
		runtime.GOOS + "/" + runtime.GOARCH,
		runtime.Version(),
	}

	res := fmt.Sprintf("%dc", runtime.NumCPU())
	if m := memTotal(); m > 0 {
		res += fmt.Sprintf("/%dg", (m+(1<<29))>>30)
	}
	parts = append(parts, res)

	if up := uptime(); up > 0 {
		parts = append(parts, "up "+formatUptime(up))
	}
	parts = append(parts, "id "+MachineID())

	fmt.Println(strings.Join(parts, "  "))
}

func infoRun(short bool) error {
	if short {
		shortInfo()
		return nil
	}

	hostname, _ := os.Hostname()
	now := time.Now()

	dateFormat := "2006-01-02"
	timeFormat := "15:04:05"
	zoneFormat := "MSTZ07:00"

	currentDate := now.Format(dateFormat)
	currentTime := now.Format(timeFormat)
	currentZone := now.Format(zoneFormat)

	user := currentUser()

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
	if m := memTotal(); m > 0 {
		fmt.Fprintf(writer, "Memory:\t%.1f GB\n", float64(m)/(1<<30))
	}
	if up := uptime(); up > 0 {
		fmt.Fprintf(writer, "Uptime:\t%s\n", formatUptime(up))
	}
	fmt.Fprintf(writer, "Machine ID:\t%s\n", MachineID())
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
	return nil
}

func NewInfoCmd() *cli.Command {
	var short bool

	return &cli.Command{
		Name:        "info",
		Description: "Show system, host and process information",
		Flags: func(fs *cli.FlagSet) {
			fs.BoolVar(&short, "short", "s", false, "Print a single line")
		},
		Examples: []string{
			meta.Name + " system info",
			meta.Name + " system info --short",
		},
		Run: func(args []string) error { return infoRun(short) },
	}
}
