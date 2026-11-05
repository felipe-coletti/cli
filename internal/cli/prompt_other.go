//go:build !unix

package cli

import "errors"

// ErrNoTerminal is returned when there is no terminal to prompt on.
var ErrNoTerminal = errors.New("passphrase prompt is not supported on this platform")

// ReadPassword is not implemented outside Unix without extra dependencies;
// use --key-file or the environment variable instead.
func ReadPassword(prompt string) ([]byte, error) {
	return nil, ErrNoTerminal
}
