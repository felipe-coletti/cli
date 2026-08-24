package crypto

import (
	"cli/internal/cli"
	"cli/internal/crypto"
	"cli/internal/meta"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
)

func EncryptRun(args []string) {
	fs := flag.NewFlagSet("encrypt", flag.ExitOnError)

	cipher := fs.String("cipher", "aes", "Cipher type (aes, xor, rot13)")
	fs.StringVar(cipher, "c", "aes", "Cipher type (short)")

	key := fs.String("key", "", "Encryption key")
	fs.StringVar(key, "k", "", "Encryption key (short)")

	input := fs.String(
		"input",
		"",
		"Path to input file to encrypt (if omitted, reads from stdin)",
	)
	fs.StringVar(
		input,
		"i",
		"",
		"Path to input file to encrypt (short)",
	)

	output := fs.String(
		"output",
		"",
		"Path to output file (if omitted, writes to stdout)",
	)
	fs.StringVar(
		output,
		"o",
		"",
		"Path to output file (short)",
	)

	force := fs.Bool(
		"force",
		false,
		"Overwrite output file if it exists",
	)
	fs.BoolVar(
		force,
		"f",
		false,
		"Overwrite output file (short)",
	)

	fs.Parse(cli.PreprocessArgs(args))

	if *key == "" {
		fmt.Fprintln(os.Stderr, "Error: --key (-k) is required")
		fmt.Fprintln(os.Stderr)
		EncryptHelp()
		os.Exit(1)
	}

	inputValue, err := crypto.ReadInput(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	result, err := crypto.Encrypt(*cipher, *key, inputValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *output != "" {
		if err := crypto.WriteFile(*output, result, *force); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		return
	}

	if err := crypto.WriteBase64(result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
