package crypto

import "cli/internal/cli"

func NewCryptoCmd() *cli.Command {
	return &cli.Command{
		Name:        "cryptography",
		Aliases:     []string{"crypto"},
		Description: "Cryptography related commands",
		Subcommands: []*cli.Command{
			NewEncryptCmd(),
			NewDecryptCmd(),
		},
	}
}
