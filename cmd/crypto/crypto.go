package crypto

import "cli/internal/cli"

func NewCryptoCmd() *cli.Command {
	return &cli.Command{
		Name:        "crypto",
		Aliases:     []string{"crypt"},
		Description: "Cryptography related commands",
		Subcommands: []*cli.Command{
			NewEncryptCmd(),
			NewDecryptCmd(),
		},
	}
}
