package crypto

import (
	"cli/internal/cli"
	"cli/internal/crypto"
	"cli/internal/meta"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

func EncryptRun(args []string) {
	fs := flag.NewFlagSet("encrypt", flag.ExitOnError)

	cipher := fs.String("cipher", "aes", "Cipher type (aes, xor, rot13)")
	fs.StringVar(cipher, "c", "aes", "Cipher type (short)")

	key := fs.String("key", "", "Encryption key")
	fs.StringVar(key, "k", "", "Encryption key (short)")

	input := fs.String("input", "", "Path to input file to encrypt (if omitted, reads from stdin)")
	fs.StringVar(input, "i", "", "Path to input file to encrypt (short)")

	output := fs.String("output", "", "Path to output file (if omitted, writes to stdout)")
	fs.StringVar(output, "o", "", "Path to output file (short)")

	force := fs.Bool("force", false, "Overwrite output file if it exists")
	fs.BoolVar(force, "F", false, "Overwrite output file (short)")

	fs.Parse(cli.PreprocessArgs(args))

	if *key == "" {
		fmt.Fprintln(os.Stderr, "Error: --key (-k) is required")
		fmt.Fprintln(os.Stderr)

		EncryptHelp()
		os.Exit(1)
	}

	if *input == "" {
		*input = "-"
	}

	if *output == "" {
		*output = "-"
	}

	in, err := cli.OpenInput(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer in.Close()

	inputData, err := io.ReadAll(in)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Error reading input: %v\n",
			err,
		)
		os.Exit(1)
	}

	if len(inputData) == 0 {
		fmt.Fprintln(os.Stderr, "Error: input is empty")
		os.Exit(1)
	}

	result, err := crypto.Encrypt(
		*cipher,
		*key,
		inputData,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *output != "-" {
		if _, err := os.Stat(*output); err == nil && !*force {
			fmt.Fprintf(
				os.Stderr,
				"Error: output file '%s' already exists. Use --force to overwrite.\n",
				*output,
			)
			os.Exit(1)
		}
	}

	if *output == "-" {
		if !cli.IsOutputPiped() {
			fmt.Println(
				base64.StdEncoding.EncodeToString(result),
			)
			return
		}

		out := os.Stdout

		if _, err := out.Write(result); err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Error writing output: %v\n",
				err,
			)
			os.Exit(1)
		}

		return
	}

	out, err := cli.OpenOutput(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	if _, err := out.Write(result); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Error writing output: %v\n",
			err,
		)
		os.Exit(1)
	}
}

func EncryptHelp() {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	fmt.Fprintf(writer, "Usage: %s crypto encrypt [options]\n\n", meta.Name)
	fmt.Fprintln(writer, "Encrypt a message or file.")
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

func NewEncryptCmd() *cli.Command {
	return &cli.Command{
		Name:        "encrypt",
		Aliases:     []string{"enc"},
		Description: "Encrypt a message or file",
		Run:         EncryptRun,
		Help:        EncryptHelp,
	}
}
