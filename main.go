package main

import (
	"os"

	"cli/cmd"
)

func main() {
	os.Exit(cmd.Main(os.Args[1:]))
}
