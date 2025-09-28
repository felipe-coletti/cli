package cmd

import (
	"cli/cmd/password"
	"cli/cmd/system"
	"cli/internal/cli"
	"fmt"
	"os"
)

var commands = []*cli.Command{
	NewVersionCmd(),
    password.NewPasswordCmd(),
    system.NewSystemCmd(),
}

func dispatch(commands []*cli.Command, args []string) {
    if len(args) == 0 {
        ShowHelp(commands)
        
		return
    }

    input := args[0]
    rest := args[1:]

    for _, cmd := range commands {
        if cmd.Name == input || contains(cmd.Aliases, input) {
            if len(cmd.Subcommands) > 0 && len(rest) > 0 {
                dispatch(cmd.Subcommands, rest)
                
				return
            }

            cmd.Run(rest)
            
			return
        }
    }

    fmt.Printf("Unknown command: %s\n\n", input)

    ShowHelp(commands)
}

func Root() {
    args := os.Args[1:]

    if len(args) == 0 || args[0] == "help" {
        ShowHelp(commands, args[1:]...)

        return
    }

    dispatch(commands, args)
}
