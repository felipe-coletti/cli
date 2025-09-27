package cmd

import (
	"cli/internal/cli"
	"fmt"
)

func ShowHelp(commands []*cli.Command, args ...string) {
	if len(args) == 0 {
		fmt.Println("Usage: mycli <command> [options]")
		fmt.Println("Available commands:")

		for _, cmd := range commands {
			fmt.Printf("  %-12s %s\n", cmd.Name, cmd.Description)
		}

		return
	}

	input := args[0]

	for _, cmd := range commands {
		if cmd.Name == input || contains(cmd.Aliases, input) {
			fmt.Printf("Usage: mycli %s [subcommand]\n", cmd.Name)
			fmt.Println(cmd.Description)

			if len(cmd.Subcommands) > 0 {
				fmt.Println("\nSubcommands:")

				for _, sub := range cmd.Subcommands {
					fmt.Printf("  %-12s %s\n", sub.Name, sub.Description)
				}
			}

			return
		}
	}

	fmt.Printf("Unknown command: %s\n\n", input)

	ShowHelp(commands)
}

func contains(list []string, val string) bool {
	for _, v := range list {
		if v == val {
			return true
		}
	}

	return false
}
