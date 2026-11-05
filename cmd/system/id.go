package system

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// machineSecret returns a stable per-machine identifier, preferring the OS
// machine id (stable across reboots, hostname changes and rebuilds).
func machineSecret() string {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(p); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				return s
			}
		}
	}
	host, _ := os.Hostname()
	return host + "|" + runtime.GOOS + "|" + runtime.GOARCH
}

// MachineID is an app-specific fingerprint: HMAC-SHA256 keyed by the CLI
// name, so the raw machine id is never exposed (as systemd recommends) and
// the value cannot be correlated with other tools that hash the same id.
func MachineID() string {
	mac := hmac.New(sha256.New, []byte(meta.Name+"/machine-id/v1"))
	mac.Write([]byte(machineSecret()))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

func NewIDCmd() *cli.Command {
	return &cli.Command{
		Name:        "id",
		Aliases:     []string{"fingerprint"},
		Description: "Print a stable fingerprint of this machine",
		Long: `
It is derived from the OS machine id (or hostname as a fallback) with an
app-specific HMAC, so it does not reveal the underlying id.`,
		Run: func(args []string) error {
			fmt.Println(MachineID())
			return nil
		},
	}
}
