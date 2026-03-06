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

func DecryptRun(args []string) {
	fs := flag.NewFlagSet("decrypt", flag.ExitOnError)

	cipher := fs.String("cipher", "aes", "Cipher type (aes, xor, rot13)")
	fs.StringVar(cipher, "c", "aes", "Cipher type (short)")

	key := fs.String("key", "", "Decryption key")
	fs.StringVar(key, "k", "", "Decryption key (short)")

	message := fs.String("message", "", "Message to decrypt")
	fs.StringVar(message, "m", "", "Message to decrypt (short)")

	fs.Parse(cli.PreprocessArgs(args))

	if *key == "" || *message == "" {
		fmt.Fprintf(os.Stderr, "Error: --key and --message are required\n\n")
		DecryptHelp()
		os.Exit(1)
	}

	result, err := crypto.Decrypt(*cipher, *key, *message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(result)
}

func DecryptHelp() {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	fmt.Fprintf(writer, "Usage: %s crypto decrypt [options]\n\n", meta.Name)
	fmt.Fprintln(writer, "Decrypt a message using specified cipher")
	fmt.Fprintln(writer, "\nOptions:")
	fmt.Fprintln(writer, "  --cipher, -c <type>\tCipher type: aes, xor, rot13 (default: aes)")
	fmt.Fprintln(writer, "  --key, -k <key>\tDecryption key (required)")
	fmt.Fprintln(writer, "  --message, -m <msg>\tMessage to decrypt (required)")
	fmt.Fprintln(writer, "\nExamples:")
	fmt.Fprintf(writer, "  %s crypto decrypt --key mykey --message hello\n", meta.Name)
	fmt.Fprintf(writer, "  %s crypto decrypt -c xor -k mykey -m hello\n", meta.Name)
}

func NewDecryptCmd() *cli.Command {
	return &cli.Command{
		Name:        "decrypt",
		Aliases:     []string{"dec"},
		Description: "Decrypt a message",
		Run:         DecryptRun,
		Help:        DecryptHelp,
	}
}
