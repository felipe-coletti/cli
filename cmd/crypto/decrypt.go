package crypto

import (
	"cli/internal/cli"
	"cli/internal/crypto"
	"cli/internal/meta"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

func DecryptRun(args []string) {
	fs := flag.NewFlagSet("decrypt", flag.ExitOnError)

	cipher := fs.String("cipher", "aes", "Cipher type (aes, xor, rot13)")
	fs.StringVar(cipher, "c", "aes", "Cipher type (short)")

	key := fs.String("key", "", "Decryption key")
	fs.StringVar(key, "k", "", "Decryption key (short)")

	input := fs.String(
		"input",
		"",
		"Path to input file to decrypt (if omitted, reads from stdin)",
	)
	fs.StringVar(
		input,
		"i",
		"",
		"Path to input file to decrypt (short)",
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
		DecryptHelp()
		os.Exit(1)
	}

	inputValue, err := crypto.ReadInput(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *input == "" {
		inputValue, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(inputValue)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid Base64 input: %v\n", err)
			os.Exit(1)
		}
	}

	result, err := crypto.Decrypt(*cipher, *key, inputValue)
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

	if _, err := os.Stdout.Write(result); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
}

func DecryptHelp() {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	fmt.Fprintf(writer, "Usage: %s crypto decrypt [options]\n\n", meta.Name)
	fmt.Fprintln(writer, "Decrypt a message or file.")
	fmt.Fprintln(writer, "\nOptions:")
	fmt.Fprintln(writer, "  --cipher, -c <type>\tCipher type: aes, xor, rot13, caesar, vigenere (default: aes)")
	fmt.Fprintln(writer, "  --key, -k <key>\tDecryption key (required)")
	fmt.Fprintln(writer, "  --input, -i <path>\tInput file (optional, reads from stdin if omitted)")
	fmt.Fprintln(writer, "  --output, -o <path>\tOutput file (optional, writes to stdout if omitted)")
	fmt.Fprintln(writer, "  --force, -f\t\tOverwrite output file if it exists")
	fmt.Fprintln(writer, "\nExamples:")
	fmt.Fprintf(writer, "  echo \"encrypted\" | %s crypto decrypt -k mykey\n", meta.Name)
	fmt.Fprintf(writer, "  %s crypto decrypt -k mykey -i secret.enc -o original.txt\n", meta.Name)
	fmt.Fprintf(writer, "  cat secret.enc | %s crypto decrypt -k mykey > result.txt\n", meta.Name)
}

func NewDecryptCmd() *cli.Command {
	return &cli.Command{
		Name:        "decrypt",
		Aliases:     []string{"dec"},
		Description: "Decrypt a message or file",
		Run:         DecryptRun,
		Help:        DecryptHelp,
	}
}
