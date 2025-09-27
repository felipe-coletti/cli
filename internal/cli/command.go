package cli

import "slices"

type Command struct {
	Name        string
	Aliases     []string
	Description string
	Run         func(args []string)
	Help        func()
	Subcommands []*Command
}

func (c Command) Matches(name string) bool {
	if c.Name == name {
		return true
	}
	
	return slices.Contains(c.Aliases, name)
}