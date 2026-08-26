package archive

import (
	"cli/internal/archive"
	"cli/internal/cli"
	"cli/internal/meta"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
)

func ExtractRun(args []string) {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)

	input := fs.String("input", ".", "Input directory")
	fs.StringVar(input, "i", ".", "Input directory (short)")

	output := fs.String("output", ".", "Output directory")
	fs.StringVar(output, "o", ".", "Output directory (short)")

	format := fs.String("format", "tar", "Archive format (tar)")
	fs.StringVar(format, "f", "tar", "Archive format (short)")

	force := fs.Bool("force", false, "Overwrite output file if it exists")
	fs.BoolVar(force, "F", false, "Overwrite output file (short)")

	fs.Parse(cli.PreprocessArgs(args))

	if *input == "" {
		fmt.Fprintln(
			os.Stderr,
			"Error: --input (-i) is required",
		)
		fmt.Fprintln(os.Stderr)

		ExtractHelp()
		os.Exit(1)
	}

	if *output == "" {
		fmt.Fprintln(
			os.Stderr,
			"Error: --output (-o) is required",
		)
		fmt.Fprintln(os.Stderr)

		ExtractHelp()
		os.Exit(1)
	}

	file, err := os.Open(*input)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Error opening input archive: %v\n",
			err,
		)
		os.Exit(1)
	}
	defer file.Close()

	options := archive.ExtractOptions{
		Force: *force,
	}

	if err := archive.Extract(
		*format,
		file,
		*output,
		options,
	); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Error: %v\n",
			err,
		)
		os.Exit(1)
	}
}

func ExtractHelp() {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	fmt.Fprintf(writer, "Usage: %s archive extract [options]\n\n", meta.Name)
	fmt.Fprintln(writer, "Extract archives.")
	fmt.Fprintln(writer, "\nOptions:")
	fmt.Fprintln(writer, "  --cipher, -c <type>\tCipher type: aes, xor, rot13, caesar, vigenere (default: aes)")
	fmt.Fprintln(writer, "  --key, -k <key>\tEncryption key (required)")
	fmt.Fprintln(writer, "  --input, -i <path>\tInput file (optional, reads from stdin if omitted)")
	fmt.Fprintln(writer, "  --output, -o <path>\tOutput file (optional, writes to stdout if omitted)")
	fmt.Fprintln(writer, "  --force, -F\t\tOverwrite output file if it exists")
	fmt.Fprintln(writer, "\nExamples:")
	fmt.Fprintf(writer, "  echo \"secret\" | %s crypto encrypt -k mykey\n", meta.Name)
	fmt.Fprintf(writer, "  %s crypto encrypt -k mykey -i input.txt -o output.enc\n", meta.Name)
	fmt.Fprintf(writer, "  %s crypto encrypt -k mykey -i data.bin -o data.enc --force\n", meta.Name)
}

func NewExtractCmd() *cli.Command {
	return &cli.Command{
		Name:        "extract",
		Description: "Extract archives",
		Run:         ExtractRun,
		Help:        ExtractHelp,
	}
}
