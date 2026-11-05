package archive

import (
	"cli/internal/archive"
	"cli/internal/cli"
	"cli/internal/meta"
	"errors"
	"fmt"
)

type packOpts struct {
	input  string
	output string
	format string
	force  bool
}

func NewPackCmd() *cli.Command {
	o := &packOpts{}
	n := meta.Name

	return &cli.Command{
		Name:        "pack",
		Description: "Create an archive from a file or directory",
		Long:        "Symlinks are stored as links (never followed).",
		Flags: func(fs *cli.FlagSet) {
			fs.StringVar(&o.input, "input", "i", "", "path", "Input file or directory (required)")
			fs.StringVar(&o.output, "output", "o", "-", "path", "Output archive (default: stdout)")
			fs.StringVar(&o.format, "format", "f", "tar", "type", "Archive format: tar")
			fs.BoolVar(&o.force, "force", "F", false, "Overwrite output file if it exists")
		},
		Examples: []string{
			fmt.Sprintf("%s archive pack -i document.txt -o document.tar", n),
			fmt.Sprintf("%s archive pack -i ./documents -o documents.tar --force", n),
			fmt.Sprintf("%s archive pack -i ./documents | %s crypto encrypt -z -o documents.tar.%s", n, n, n),
		},
		Run: func(args []string) error { return o.run() },
	}
}

func (o *packOpts) run() error {
	if o.input == "" {
		return cli.Usage(errors.New("--input (-i) is required"))
	}

	out, err := cli.CreateOutput(o.output, o.force)
	if err != nil {
		return err
	}

	if err := archive.Pack(o.format, o.input, out); err != nil {
		out.Abort()
		return err
	}

	return out.Commit()
}
