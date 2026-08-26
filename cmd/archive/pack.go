package archive

import (
	"cli/internal/archive"
	"cli/internal/cli"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"cli/internal/meta"
)

func PackRun(args []string) {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)

	input := fs.String("input", "", "Path to file or directory to archive")
	fs.StringVar(input, "i", "", "Path to file or directory to archive (short)")

	output := fs.String("output", "", "Path to output archive")
	fs.StringVar(output, "o", "", "Path to output archive (short)")

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

		PackHelp()
		os.Exit(1)
	}

	var out *os.File

	if *output == "" {
		out = os.Stdout
	} else {
		if _, err := os.Stat(*output); err == nil && !*force {
			fmt.Fprintf(
				os.Stderr,
				"Error: output file '%s' already exists. Use --force to overwrite.\n",
				*output,
			)
			os.Exit(1)
		}

		var err error

		out, err = os.Create(*output)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Error creating output file: %v\n",
				err,
			)
			os.Exit(1)
		}

		defer out.Close()
	}

	if err := archive.Pack(*format, *input, out); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Error: %v\n",
			err,
		)
		os.Exit(1)
	}
}

func PackHelp() {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	fmt.Fprintf(writer, "Usage: %s archive pack [options]\n\n", meta.Name)
	fmt.Fprintln(writer, "Create an archive from a file or directory.")
	fmt.Fprintln(writer, "\nOptions:")
	fmt.Fprintln(writer, "  --input, -i <path>\tInput file or directory (required)")
	fmt.Fprintln(writer, "  --output, -o <path>\tOutput archive (optional, writes to stdout if omitted)")
	fmt.Fprintln(writer, "  --format, -f <type>\tArchive format: tar (default: tar)")
	fmt.Fprintln(writer, "  --force, -F\t\tOverwrite output file if it exists")
	fmt.Fprintln(writer, "\nExamples:")
	fmt.Fprintf(writer, "  %s archive pack -i document.txt -o document.tar\n", meta.Name)
	fmt.Fprintf(writer, "  %s archive pack -i ./documents -o documents.tar\n", meta.Name)
	fmt.Fprintf(writer, "  %s archive pack -i ./documents -o documents.tar --format tar --force\n", meta.Name)
}

func NewPackCmd() *cli.Command {
	return &cli.Command{
		Name:        "pack",
		Description: "Create an archive from a file or directory",
		Run:         PackRun,
		Help:        PackHelp,
	}
}
