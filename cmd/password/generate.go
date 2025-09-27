package password

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"cli/internal/password"
	"flag"
	"fmt"
)

func GenerateRun(args []string) {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)

	length := fs.Int("L", 15, "Password length")
	uppercase := fs.Bool("u", false, "Include uppercase letters")
	lowercase := fs.Bool("l", false, "Include lowercase letters")
	numbers := fs.Bool("n", false, "Include numbers")
	symbols := fs.Bool("s", false, "Include symbols")

	fs.Parse(args)

	fmt.Println(password.Generate(*length, *uppercase, *lowercase, *numbers, *symbols))
}

func GenerateHelp() {
	fmt.Printf("Usage: %s {usage} [options]\n", meta.Name)
	fmt.Println()
	fmt.Println("Generate random passwords")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --count, -C <num>       Number of passwords (default: 1)")
	fmt.Println("  --length, -L <num>      Length of each password (default: 15)")
	fmt.Println("  --uppercase, -u         Include uppercase letters")
	fmt.Println("  --lowercase, -l         Include lowercase letters")
	fmt.Println("  --numbers, -n           Include numbers")
	fmt.Println("  --symbols, -s           Include special symbols")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s {usage}    # 1 password, 15 chars, all character types\n", meta.Name)
	fmt.Printf("  %s {usage} --count 5 --length 15 --uppercase --lowercase --numbers --symbols\n", meta.Name)
	fmt.Printf("  %s {usage} gen -C 5 -L 15 -u -l -n -s\n", meta.Name)
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
