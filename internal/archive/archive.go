package archive

import (
	"fmt"
	"io"
)

type ExtractOptions struct {
	Force bool
}

func Pack(format, input string, output io.Writer) error {
	switch format {
	case "tar":
		return PackTar(input, output)

	default:
		return fmt.Errorf("unsupported archive format: %s", format)
	}
}

func Extract(format string, input io.Reader, output string, options ExtractOptions) error {
	switch format {
	case "tar":
		return ExtractTar(input, output, options)

	default:
		return fmt.Errorf("unsupported archive format: %s", format)
	}
}
