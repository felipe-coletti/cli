package cmd

import (
	"cli/cmd/password"
	"cli/cmd/system"
	"cli/internal/cli"
	"fmt"
	"os"
)

var commands = []*cli.Command{
	system.NewSystemCmd(),
	password.NewPasswordCmd(),
	NewVersionCmd(),
}

func Root() {
	if len(os.Args) < 2 {
		ShowHelp(commands)
		return
	}

	input := os.Args[1]
	args := os.Args[2:]

	if input == "help" {
		ShowHelp(commands, args...)

		return
	}

	for _, cmd := range commands {
		if cmd.Name == input || contains(cmd.Aliases, input) {
			if len(cmd.Subcommands) > 0 && len(args) > 0 {
				for _, sub := range cmd.Subcommands {
					if sub.Name == args[0] {
						sub.Run(args[1:])

						return
					}
				}

				fmt.Printf("Unknown subcommand: %s\n\n", args[0])

				ShowHelp([]*cli.Command{cmd})

				return
			}

			cmd.Run(args)

			return
		}
	}

	fmt.Printf("Unknown command: %s\n\n", input)

	ShowHelp(commands)
}
