package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"cli/internal/meta"
)

// Command is a node in the command tree. A command either has Subcommands
// (a group, like "crypto") or a Run function (a leaf, like "encrypt").
type Command struct {
	Name        string
	Aliases     []string
	Description string   // one line, shown in listings
	Long        string   // optional extra text for the help page
	Args        string   // positional arguments for the usage line, e.g. "[file]"
	Examples    []string // full command lines
	Flags       func(fs *FlagSet)
	Run         func(args []string) error
	Subcommands []*Command
}

// Matches reports whether name is the command's name or one of its aliases.
func (c *Command) Matches(name string) bool {
	return c.Name == name || slices.Contains(c.Aliases, name)
}

// Find returns the command matching name, or nil.
func Find(cmds []*Command, name string) *Command {
	for _, c := range cmds {
		if c.Matches(name) {
			return c
		}
	}
	return nil
}

// UsageError is an error caused by how the command was invoked. Execute's
// caller prints it followed by the relevant help page.
type UsageError struct {
	Err  error
	Path []string
	Cmd  *Command
	Cmds []*Command // for unknown commands: the list that was searched
}

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// Usage marks err as a usage error; Execute fills in which command it belongs to.
func Usage(err error) error { return &UsageError{Err: err} }

// Usagef is fmt.Errorf for usage errors.
func Usagef(format string, a ...any) error { return Usage(fmt.Errorf(format, a...)) }

// Execute runs the command selected by args.
func Execute(root []*Command, args []string) error {
	if len(args) > 0 && args[0] == "help" {
		return help(root, nil, args[1:])
	}
	return run(root, nil, args)
}

func isHelpFlag(s string) bool { return s == "-h" || s == "--help" }

func run(cmds []*Command, path []string, args []string) error {
	if len(args) == 0 || isHelpFlag(args[0]) {
		PrintGroupHelp(os.Stdout, path, cmds)
		return nil
	}

	c := Find(cmds, args[0])
	if c == nil {
		return &UsageError{Err: fmt.Errorf("unknown command: %s", args[0]), Path: path, Cmds: cmds}
	}
	path = append(path, c.Name)

	if c.Run == nil {
		return run(c.Subcommands, path, args[1:])
	}

	fs := NewFlagSet(c.Name)
	if c.Flags != nil {
		c.Flags(fs)
	}

	pre, err := PreprocessArgs(args[1:])
	if err != nil {
		return &UsageError{Err: err, Path: path, Cmd: c}
	}
	if err := fs.Parse(pre); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			PrintHelp(os.Stdout, path, c)
			return nil
		}
		return &UsageError{Err: err, Path: path, Cmd: c}
	}

	err = c.Run(fs.Args())

	var ue *UsageError
	if errors.As(err, &ue) && ue.Cmd == nil && ue.Cmds == nil {
		ue.Path, ue.Cmd = path, c
	}
	return err
}

func help(cmds []*Command, path []string, args []string) error {
	if len(args) == 0 {
		PrintGroupHelp(os.Stdout, path, cmds)
		return nil
	}

	c := Find(cmds, args[0])
	if c == nil {
		return &UsageError{Err: fmt.Errorf("unknown command: %s", args[0]), Path: path, Cmds: cmds}
	}
	path = append(path, c.Name)

	if c.Run == nil {
		return help(c.Subcommands, path, args[1:])
	}
	PrintHelp(os.Stdout, path, c)
	return nil
}

// ---------------------------------------------------------------------------
// Help rendering

func usagePrefix(path []string) string {
	return strings.Join(append([]string{meta.Name}, path...), " ")
}

func names(c *Command) string {
	return strings.Join(append([]string{c.Name}, c.Aliases...), ", ")
}

// PrintGroupHelp lists the commands available at path.
func PrintGroupHelp(w io.Writer, path []string, cmds []*Command) {
	fmt.Fprintf(w, "Usage: %s <command> [options]\n\n", usagePrefix(path))

	fmt.Fprintln(w, "Commands:")
	width := 0
	for _, c := range cmds {
		width = max(width, len(names(c)))
	}
	for _, c := range cmds {
		fmt.Fprintf(w, "  %-*s  %s\n", width, names(c), c.Description)
	}

	helpCmd := strings.Join(append([]string{meta.Name, "help"}, path...), " ")
	fmt.Fprintf(w, "\nRun '%s <command>' for details.\n", helpCmd)
}

// PrintHelp renders the help page of a leaf command from its flags.
func PrintHelp(w io.Writer, path []string, c *Command) {
	fs := NewFlagSet(c.Name)
	if c.Flags != nil {
		c.Flags(fs)
	}

	usage := usagePrefix(path)
	if len(fs.specs) > 0 {
		usage += " [options]"
	}
	if c.Args != "" {
		usage += " " + c.Args
	}
	fmt.Fprintf(w, "Usage: %s\n\n", usage)

	fmt.Fprintln(w, c.Description+".")
	if c.Long != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, strings.TrimSpace(c.Long))
	}
	if len(c.Aliases) > 0 {
		fmt.Fprintf(w, "\nAliases: %s\n", strings.Join(c.Aliases, ", "))
	}

	fs.printOptions(w)

	if len(c.Examples) > 0 {
		fmt.Fprintln(w, "\nExamples:")
		for _, e := range c.Examples {
			fmt.Fprintf(w, "  %s\n", e)
		}
	}
}

// PrintUsageError prints err and the help page it relates to, to stderr.
func PrintUsageError(err *UsageError) {
	fmt.Fprintf(os.Stderr, "Error: %v\n\n", err.Err)
	switch {
	case err.Cmd != nil:
		PrintHelp(os.Stderr, err.Path, err.Cmd)
	case err.Cmds != nil:
		PrintGroupHelp(os.Stderr, err.Path, err.Cmds)
	}
}
