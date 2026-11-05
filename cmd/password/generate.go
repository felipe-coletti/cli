package password

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"cli/internal/password"
	"fmt"
	"os"
	"text/tabwriter"
)

type generateOpts struct {
	count, length                          int
	uppercase, lowercase, numbers, symbols bool
}

func NewGenerateCmd() *cli.Command {
	o := &generateOpts{}
	n := meta.Name

	return &cli.Command{
		Name:        "generate",
		Aliases:     []string{"gen"},
		Description: "Generate random passwords",
		Long:        "Without character-set flags, all character types are used.",
		Flags: func(fs *cli.FlagSet) {
			fs.IntVar(&o.count, "count", "C", 1, "n", "Number of passwords")
			fs.IntVar(&o.length, "length", "L", 15, "n", "Length of each password")
			fs.BoolVar(&o.uppercase, "uppercase", "u", false, "Include uppercase letters")
			fs.BoolVar(&o.lowercase, "lowercase", "l", false, "Include lowercase letters")
			fs.BoolVar(&o.numbers, "numbers", "n", false, "Include numbers")
			fs.BoolVar(&o.symbols, "symbols", "s", false, "Include special symbols")
		},
		Examples: []string{
			fmt.Sprintf("%s password generate", n),
			fmt.Sprintf("%s password generate --count 5 --length 20 --uppercase --lowercase --numbers", n),
			fmt.Sprintf("%s password gen -C 5 -L 20 -u -l -n -s", n),
		},
		Run: func(args []string) error { return o.run() },
	}
}

func (o *generateOpts) run() error {
	if o.length <= 0 || o.count <= 0 {
		return cli.Usagef("--count and --length must be positive")
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	defer writer.Flush()

	for i := range o.count {
		pass, err := password.Generate(o.length, o.uppercase, o.lowercase, o.numbers, o.symbols)
		if err != nil {
			return err
		}
		if o.count == 1 {
			fmt.Fprintln(writer, pass)
		} else {
			fmt.Fprintf(writer, "%d\t%s\n", i+1, pass)
		}
	}
	return nil
}
