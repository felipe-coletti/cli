package password

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"cli/internal/password"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
)

func GenerateRun(args []string) {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)

	count := fs.Int("count", 1, "Password count")
	fs.IntVar(count, "C", 1, "Password count (short)")

	length := fs.Int("length", 15, "Password length")
	fs.IntVar(length, "L", 15, "Password length (short)")

	uppercase := fs.Bool("uppercase", false, "Include uppercase letters")
	fs.BoolVar(uppercase, "u", false, "Include uppercase letters (short)")

	lowercase := fs.Bool("lowercase", false, "Include lowercase letters")
	fs.BoolVar(lowercase, "l", false, "Include lowercase letters (short)")

	numbers := fs.Bool("numbers", false, "Include numbers")
	fs.BoolVar(numbers, "n", false, "Include numbers (short)")

	symbols := fs.Bool("symbols", false, "Include symbols")
	fs.BoolVar(symbols, "s", false, "Include symbols (short)")

	fs.Parse(password.PreprocessArgs(args))

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	for i := 0; i < *count; i++ {
		pass := password.Generate(*length, *uppercase, *lowercase, *numbers, *symbols)

		fmt.Fprintf(writer, "%d\t%s\n", i + 1, pass)
	}
}

func GenerateHelp() {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0)
	defer writer.Flush()

	fmt.Fprintf(writer, "Usage: %s password generate [options]\n\n", meta.Name)
	fmt.Fprintln(writer, "Generate random passwords")
	fmt.Fprintln(writer, "\nOptions:")
	fmt.Fprintln(writer, "  --count, -C <num>\tNumber of passwords (default: 1)")
	fmt.Fprintln(writer, "  --length, -L <num>\tLength of each password (default: 15)")
	fmt.Fprintln(writer, "  --uppercase, -u\tInclude uppercase letters")
	fmt.Fprintln(writer, "  --lowercase, -l\tInclude lowercase letters")
	fmt.Fprintln(writer, "  --numbers, -n\tInclude numbers")
	fmt.Fprintln(writer, "  --symbols, -s\tInclude special symbols")
	fmt.Fprintln(writer, "\nExamples:")
	fmt.Fprintf(writer, "  %s password generate    # 1 password, 15 chars, all character types\n", meta.Name)
	fmt.Fprintf(writer, "  %s password generate --count 5 --length 15 --uppercase --lowercase --numbers --symbols\n", meta.Name)
	fmt.Fprintf(writer, "  %s password gen -C 5 -L 15 -u -l -n -s\n", meta.Name)
}

func NewGenerateCmd() *cli.Command {
	return &cli.Command{
		Name:        "generate",
		Aliases:     []string{"gen"},
		Description: "Generate random passwords",
		Run:         GenerateRun,
		Help:        GenerateHelp,
	}
}
