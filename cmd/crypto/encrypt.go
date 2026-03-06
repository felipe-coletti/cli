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

	message := fs.String("message", "", "Message to encrypt")
	fs.StringVar(message, "m", "", "Message to encrypt (short)")

	fs.Parse(cli.PreprocessArgs(args))

	if *key == "" || *message == "" {
		fmt.Fprintf(os.Stderr, "Error: --key and --message are required\n\n")
		EncryptHelp()
		os.Exit(1)
	}

	result, err := crypto.Encrypt(*cipher, *key, *message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(result)
}

func EncryptHelp() {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	fmt.Fprintf(writer, "Usage: %s crypto encrypt [options]\n\n", meta.Name)
	fmt.Fprintln(writer, "Encrypt a message using specified cipher")
	fmt.Fprintln(writer, "\nOptions:")
	fmt.Fprintln(writer, "  --cipher, -c <type>\tCipher type: aes, xor, rot13 (default: aes)")
	fmt.Fprintln(writer, "  --key, -k <key>\tEncryption key (required)")
	fmt.Fprintln(writer, "  --message, -m <msg>\tMessage to encrypt (required)")
	fmt.Fprintln(writer, "\nExamples:")
	fmt.Fprintf(writer, "  %s crypto encrypt --key mykey --message hello\n", meta.Name)
	fmt.Fprintf(writer, "  %s crypto encrypt -c xor -k mykey -m hello\n", meta.Name)
}

func NewEncryptCmd() *cli.Command {
	return &cli.Command{
		Name:        "encrypt",
		Aliases:     []string{"enc"},
		Description: "Encrypt a message",
		Run:         EncryptRun,
		Help:        EncryptHelp,
	}
}
