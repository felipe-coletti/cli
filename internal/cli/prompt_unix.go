//go:build unix

package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

// ErrNoTerminal is returned when there is no terminal to prompt on.
var ErrNoTerminal = errors.New("no terminal available to prompt for a passphrase")

// ReadPassword prompts on the controlling terminal (/dev/tty), so it works
// even when stdin/stdout carry data. Echo is disabled with stty, which keeps
// the binary free of non-stdlib dependencies.
func ReadPassword(prompt string) ([]byte, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, ErrNoTerminal
	}
	defer tty.Close()

	if err := stty(tty, "-echo"); err != nil {
		return nil, ErrNoTerminal
	}

	// Restore echo if the user hits Ctrl+C at the prompt.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-sigs:
			stty(tty, "echo")
			fmt.Fprintln(tty)
			os.Exit(130)
		case <-done:
		}
	}()
	defer func() {
		close(done)
		signal.Stop(sigs)
		stty(tty, "echo")
	}()

	fmt.Fprint(tty, prompt)
	line, err := bufio.NewReader(tty).ReadString('\n')
	fmt.Fprintln(tty)
	if err != nil && line == "" {
		return nil, fmt.Errorf("read passphrase: %w", err)
	}

	return []byte(strings.TrimRight(line, "\r\n")), nil
}

func stty(tty *os.File, arg string) error {
	cmd := exec.Command("stty", arg)
	cmd.Stdin = tty
	return cmd.Run()
}
