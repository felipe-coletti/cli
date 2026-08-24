package crypto

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

func IsInputPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) == 0
}

func ReadInput(file string) ([]byte, error) {
	var (
		data []byte
		err  error
	)

	if file != "" {
		data, err = os.ReadFile(file)
	} else {
		if !IsInputPiped() {
			return nil, errors.New(
				"no input provided. Use --file or pipe data to stdin",
			)
		}

		data, err = io.ReadAll(os.Stdin)
	}

	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, errors.New("input is empty")
	}

	return data, nil
}

func WriteFile(path string, data []byte, force bool) error {
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf(
			"output file '%s' already exists. Use --force to overwrite",
			path,
		)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}

	return nil
}

func WriteBase64(data []byte) error {
	encoded := base64.StdEncoding.EncodeToString(data)

	if _, err := fmt.Fprintln(os.Stdout, encoded); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}
