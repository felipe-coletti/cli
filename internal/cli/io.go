package cli

import (
	"fmt"
	"io"
	"os"
)

type nopCloser struct {
	io.Reader
	io.Writer
}

func (n nopCloser) Close() error {
	return nil
}

func OpenInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return nopCloser{Reader: os.Stdin}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}

	return file, nil
}

func OpenOutput(path string) (io.WriteCloser, error) {
	if path == "-" {
		return nopCloser{Writer: os.Stdout}, nil
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create output: %w", err)
	}

	return file, nil
}

func IsInputPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return stat.Mode()&os.ModeCharDevice == 0
}

func IsOutputPiped() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return stat.Mode()&os.ModeCharDevice == 0
}
