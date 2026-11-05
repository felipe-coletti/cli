package cli

import (
	"fmt"
	"io"
	"os"
)

type nopReadCloser struct{ io.Reader }

func (nopReadCloser) Close() error { return nil }

// OpenInput opens path for reading; "-" means stdin.
func OpenInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return nopReadCloser{os.Stdin}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}

	return file, nil
}

// IsOutputPiped reports whether stdout is not a terminal.
func IsOutputPiped() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return stat.Mode()&os.ModeCharDevice == 0
}
