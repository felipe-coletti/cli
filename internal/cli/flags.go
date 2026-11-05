package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
)

// FlagSet wraps flag.FlagSet so every option can have a long and a short
// name and the help page is generated from the definitions, so it can never
// drift from the actual flags.
type FlagSet struct {
	*flag.FlagSet
	specs []flagSpec
}

type flagSpec struct {
	long, short string
	arg         string // placeholder for the value, empty for booleans
	usage       string
	def         string // default shown in help, empty for none
}

// NewFlagSet returns a FlagSet that reports errors instead of exiting.
func NewFlagSet(name string) *FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return &FlagSet{FlagSet: fs}
}

func (f *FlagSet) add(s flagSpec) { f.specs = append(f.specs, s) }

// StringVar defines --long and (optionally) -short. arg names the value in help.
func (f *FlagSet) StringVar(p *string, long, short, value, arg, usage string) {
	f.FlagSet.StringVar(p, long, value, usage)
	if short != "" {
		f.FlagSet.StringVar(p, short, value, usage)
	}
	def := value
	if value == "-" {
		def = "" // stdin/stdout; the usage text says so
	}
	f.add(flagSpec{long: long, short: short, arg: arg, usage: usage, def: def})
}

// IntVar defines an integer option.
func (f *FlagSet) IntVar(p *int, long, short string, value int, arg, usage string) {
	f.FlagSet.IntVar(p, long, value, usage)
	if short != "" {
		f.FlagSet.IntVar(p, short, value, usage)
	}
	def := ""
	if value != 0 {
		def = strconv.Itoa(value)
	}
	f.add(flagSpec{long: long, short: short, arg: arg, usage: usage, def: def})
}

// BoolVar defines a boolean switch.
func (f *FlagSet) BoolVar(p *bool, long, short string, value bool, usage string) {
	f.FlagSet.BoolVar(p, long, value, usage)
	if short != "" {
		f.FlagSet.BoolVar(p, short, value, usage)
	}
	def := ""
	if value {
		def = "true"
	}
	f.add(flagSpec{long: long, short: short, usage: usage, def: def})
}

func (f *FlagSet) printOptions(w io.Writer) {
	fmt.Fprintln(w, "\nOptions:")

	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	row := func(s flagSpec) {
		left := "--" + s.long
		if s.short != "" {
			left += ", -" + s.short
		}
		if s.arg != "" {
			left += " <" + s.arg + ">"
		}

		// Usage may span several lines: continuation lines are indented.
		lines := strings.Split(s.usage, "\n")
		if s.def != "" {
			lines[len(lines)-1] += " (default: " + s.def + ")"
		}
		fmt.Fprintf(tw, "  %s\t%s\n", left, lines[0])
		for _, l := range lines[1:] {
			fmt.Fprintf(tw, "  \t%s\n", l)
		}
	}

	for _, s := range f.specs {
		row(s)
	}
	row(flagSpec{long: "help", short: "h", usage: "Show this help"})
	tw.Flush()
}
