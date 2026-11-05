package metadata

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"cli/internal/cli"
	"cli/internal/meta"
	"cli/internal/metadata"
)

type stripOpts struct {
	input           string
	output          string
	inPlace         bool
	force           bool
	all             bool
	copyUnsupported bool
	jobs            int
	verbose         bool
}

func NewStripCmd() *cli.Command {
	o := &stripOpts{}
	n := meta.Name

	return &cli.Command{
		Name:        "strip",
		Aliases:     []string{"clean"},
		Description: "Remove metadata from a file or a whole directory",
		Long: formatsHelp + `

By default a cleaned copy is written to --output; --in-place replaces the
originals (atomically). The EXIF orientation is kept unless --all is given,
otherwise many phone photos would show up rotated.

Only known metadata is removed: information inside the content itself (text
in a document, faces or plates in a photo) is not.`,
		Flags: func(fs *cli.FlagSet) {
			fs.StringVar(&o.input, "input", "i", "", "path", "File or directory to clean (required)")
			fs.StringVar(&o.output, "output", "o", "", "path", "Output file or directory")
			fs.BoolVar(&o.inPlace, "in-place", "", false, "Replace the original files instead of writing copies")
			fs.BoolVar(&o.force, "force", "F", false, "Overwrite existing output files")
			fs.BoolVar(&o.all, "all", "a", false, "Also remove the EXIF orientation")
			fs.BoolVar(&o.copyUnsupported, "copy-unsupported", "", false, "Copy unsupported files to the output unchanged\n(they may still contain metadata)")
			fs.IntVar(&o.jobs, "jobs", "j", 0, "n", "Parallel workers in directory mode (default: CPU count)")
			fs.BoolVar(&o.verbose, "verbose", "v", false, "Also report clean and skipped files")
		},
		Examples: []string{
			fmt.Sprintf("%s metadata strip -i photo.jpg -o photo-clean.jpg", n),
			fmt.Sprintf("%s metadata strip -i ./photos -o ./photos-clean", n),
			fmt.Sprintf("%s metadata strip -i ./photos --in-place", n),
		},
		Run: func(args []string) error { return o.run() },
	}
}

type stats struct {
	mu                                    sync.Mutex
	cleaned, clean, skipped, copied, fail int
}

func (o *stripOpts) run() error {
	switch {
	case o.input == "":
		return cli.Usagef("--input (-i) is required")
	case o.output == "" && !o.inPlace:
		return cli.Usagef("pass --output (-o) or --in-place")
	case o.output != "" && o.inPlace:
		return cli.Usagef("--output and --in-place are mutually exclusive")
	}

	fi, err := os.Stat(o.input)
	if err != nil {
		return err
	}

	var jobs []cli.Job
	if fi.IsDir() {
		jobs, err = cli.PlanTree(o.input, o.output, func(rel string) (string, bool) { return rel, true })
		if err != nil {
			return err
		}
	} else {
		out := o.output
		if out != "" {
			if ofi, err := os.Stat(out); err == nil && ofi.IsDir() {
				out = filepath.Join(out, filepath.Base(o.input))
			}
		}
		jobs = []cli.Job{{In: o.input, Out: out}}
	}
	if o.inPlace {
		for i := range jobs {
			jobs[i].Out = jobs[i].In
		}
	}

	st := &stats{}

	if !fi.IsDir() {
		o.verbose = true // always say what happened to a single file
		if err := o.process(jobs[0], st); err != nil {
			return err
		}
		if st.skipped > 0 {
			return fmt.Errorf("%s: %w", o.input, metadata.ErrUnsupported)
		}
		return nil
	}

	errs := cli.RunPool(jobs, o.jobs, func(j cli.Job) error { return o.process(j, st) })
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "%d cleaned, %d already clean, %d unsupported", st.cleaned, st.clean, st.skipped+st.copied)
	if st.copied > 0 {
		fmt.Fprintf(os.Stderr, " (%d copied unchanged)", st.copied)
	}
	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, ", %d failed", len(errs))
	}
	fmt.Fprintln(os.Stderr)

	if len(errs) > 0 {
		return fmt.Errorf("%d of %d files failed", len(errs), len(jobs))
	}
	return nil
}

func (o *stripOpts) log(st *stats, format string, a ...any) {
	st.mu.Lock()
	defer st.mu.Unlock()
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

func (o *stripOpts) count(st *stats, field *int) {
	st.mu.Lock()
	*field++
	st.mu.Unlock()
}

func (o *stripOpts) process(j cli.Job, st *stats) error {
	a, fi, err := analyze(j.In, metadata.Options{KeepOrientation: !o.all})

	if isUnsupported(err) {
		if !o.inPlace && o.copyUnsupported && fi != nil {
			if err := copyFile(j.In, j.Out, fi.Mode().Perm(), o.force); err != nil {
				return err
			}
			o.count(st, &st.copied)
			if o.verbose {
				o.log(st, "copied   %s (unsupported, unchanged)", j.In)
			}
			return nil
		}
		o.count(st, &st.skipped)
		if o.verbose {
			o.log(st, "skipped  %s (unsupported)", j.In)
		}
		return nil
	}
	if err != nil {
		return err
	}
	defer a.Close()

	res := a.res
	if !res.Changed() {
		o.count(st, &st.clean)
		if o.verbose {
			o.log(st, "clean    %s", j.In)
		}
		if o.inPlace {
			return nil // nothing to rewrite
		}
	}

	perm := a.fi.Mode().Perm()
	switch {
	case res.Data == nil && o.inPlace:
		// MP4/MOV/HEIC: overwrite the few metadata bytes directly, no copy
		// of a possibly multi-gigabyte file.
		f, err := os.OpenFile(j.In, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		if err := metadata.PatchInPlace(f, res.Patches); err != nil {
			f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	case res.Data == nil:
		err = writeWith(j.Out, perm, o.force, func(w io.Writer) error {
			return metadata.ApplyPatches(w, a.f, a.fi.Size(), res.Patches)
		})
	default:
		err = writeWith(j.Out, perm, o.force || o.inPlace, func(w io.Writer) error {
			_, err := w.Write(res.Data)
			return err
		})
	}
	if err != nil {
		return err
	}

	if res.Changed() {
		o.count(st, &st.cleaned)
		o.log(st, "cleaned  %s (%s)", j.In, kindList(res.Removed))
	}
	return nil
}

// writeWith writes path atomically with the content produced by fill.
func writeWith(path string, perm os.FileMode, force bool, fill func(io.Writer) error) error {
	if err := cli.MkParent(path); err != nil {
		return err
	}
	out, err := cli.CreateAtomic(path, force, perm)
	if err != nil {
		return err
	}
	if err := fill(out); err != nil {
		out.Abort()
		return err
	}
	return out.Commit()
}

func copyFile(in, out string, perm os.FileMode, force bool) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeWith(out, perm, force, func(w io.Writer) error {
		_, err := io.Copy(w, f)
		return err
	})
}
