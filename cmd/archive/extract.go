package archive

import (
	"cli/internal/archive"
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
)

type extractOpts struct {
	input  string
	output string
	format string
	force  bool
}

func NewExtractCmd() *cli.Command {
	o := &extractOpts{}
	n := meta.Name

	return &cli.Command{
		Name:        "extract",
		Description: "Extract an archive into a directory",
		Long: `
Entries can never be written outside the output directory: paths with ".."
are rejected and symlinks pointing outside it are refused.`,
		Flags: func(fs *cli.FlagSet) {
			fs.StringVar(&o.input, "input", "i", "-", "path", "Input archive (default: stdin)")
			fs.StringVar(&o.output, "output", "o", ".", "path", "Output directory")
			fs.StringVar(&o.format, "format", "f", "tar", "type", "Archive format: tar")
			fs.BoolVar(&o.force, "force", "F", false, "Overwrite existing files")
		},
		Examples: []string{
			fmt.Sprintf("%s archive extract -i documents.tar -o ./restore", n),
			fmt.Sprintf("%s crypto decrypt -i docs.tar.%s | %s archive extract -o ./docs", n, n, n),
		},
		Run: func(args []string) error { return o.run() },
	}
}

func (o *extractOpts) run() error {
	in, err := cli.OpenInput(o.input)
	if err != nil {
		return err
	}
	defer in.Close()

	return archive.Extract(o.format, in, o.output, archive.ExtractOptions{Force: o.force})
}
