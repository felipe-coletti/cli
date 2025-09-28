package cmd

import (
	"slices"
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
)

func ShowHelp(commands []*cli.Command, args ...string) {
    if len(args) == 0 {
        fmt.Printf("Usage: %s <command> [options]\n\n", meta.Name)
        fmt.Println("Available commands:")
        for _, cmd := range commands {
            fmt.Printf("  %-12s %s\n", cmd.Name, cmd.Description)
        }
        return
    }

    input := args[0]

    for _, cmd := range commands {
        if cmd.Name == input || slices.Contains(cmd.Aliases, input) {
            if len(args) > 1 {
                ShowHelp(cmd.Subcommands, args[1:]...)
                return
            }

            if cmd.Help != nil {
                cmd.Help()
            } else {
                fmt.Printf("Usage: %s %s\n\n", meta.Name, cmd.Name)
                fmt.Printf("%s\n", cmd.Description)

                if len(cmd.Subcommands) > 0 {
                    fmt.Println("\nSubcommands:")
                    for _, sub := range cmd.Subcommands {
                        fmt.Printf("  %-12s %s\n", sub.Name, sub.Description)
                    }
                }
            }

            return
        }
    }

    fmt.Printf("Unknown command: %s\n\n", input)
    ShowHelp(commands)
}
