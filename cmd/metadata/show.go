package metadata

import (
	"fmt"
	"os"
	"text/tabwriter"

	"cli/internal/cli"
	"cli/internal/meta"
	"cli/internal/metadata"
)

type showOpts struct {
	all     bool
	verbose bool
}

func NewShowCmd() *cli.Command {
	o := &showOpts{}
	n := meta.Name

	return &cli.Command{
		Name:        "show",
		Aliases:     []string{"ls"},
		Description: "List the metadata in files or directories",
		Long:        "Shows exactly what 'strip' would remove and keep.\n" + formatsHelp,
		Args:        "<file|dir>...",
		Flags: func(fs *cli.FlagSet) {
			fs.BoolVar(&o.all, "all", "a", false, "Preview stripping with --all (orientation removed too)")
			fs.BoolVar(&o.verbose, "verbose", "v", false, "Also list clean and unsupported files")
		},
		Examples: []string{
			fmt.Sprintf("%s metadata show photo.jpg", n),
			fmt.Sprintf("%s metadata show ./photos report.docx", n),
		},
		Run: o.run,
	}
}

func (o *showOpts) run(args []string) error {
	if len(args) == 0 {
		return cli.Usagef("pass at least one file or directory")
	}
	files, err := collect(args)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	defer w.Flush()

	var dirty, clean, unsupported, failed int
	for _, path := range files {
		a, _, err := analyze(path, metadata.Options{KeepOrientation: !o.all})
		if isUnsupported(err) {
			unsupported++
			if o.verbose {
				fmt.Fprintf(w, "%s\tunsupported\n", path)
			}
			continue
		}
		if err != nil {
			failed++
			fmt.Fprintf(w, "%s\terror: %v\n", path, err)
			continue
		}
		res := a.res
		a.Close()

		if !res.Changed() {
			clean++
			if o.verbose {
				fmt.Fprintf(w, "%s\t%s, no metadata\n", path, res.Format)
			}
			continue
		}

		dirty++
		fmt.Fprintf(w, "%s\t%s\n", path, res.Format)
		for _, it := range res.Removed {
			fmt.Fprintf(w, "  remove  %s\t%s\n", it.Kind, it.Detail)
		}
		for _, it := range res.Kept {
			fmt.Fprintf(w, "  keep    %s\t%s\n", it.Kind, it.Detail)
		}
	}

	if len(files) > 1 {
		fmt.Fprintf(w, "\n%d files: %d with metadata, %d clean, %d unsupported", len(files), dirty, clean, unsupported)
		if failed > 0 {
			fmt.Fprintf(w, ", %d failed", failed)
		}
		fmt.Fprintln(w)
	}
	if failed > 0 {
		w.Flush()
		return fmt.Errorf("%d files could not be read", failed)
	}
	return nil
}
