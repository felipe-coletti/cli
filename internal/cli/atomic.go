package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Output is a destination that only becomes visible once Commit succeeds.
// For files this is implemented with a temporary file in the same directory
// followed by fsync + rename, so a crash or an error never leaves a
// half-written file behind. Stdout cannot be rolled back: Abort is a no-op.
type Output interface {
	io.Writer
	Commit() error
	Abort()
}

// CreateOutput opens path for writing ("-" means stdout). Unless force is
// set it fails if path already exists.
func CreateOutput(path string, force bool) (Output, error) {
	if path == "-" {
		return stdoutOutput{}, nil
	}
	return CreateAtomic(path, force, 0o644)
}

type stdoutOutput struct{}

func (stdoutOutput) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdoutOutput) Commit() error               { return nil }
func (stdoutOutput) Abort()                      {}

// AtomicFile writes to a temporary file and renames it over the target on Commit.
type AtomicFile struct {
	f     *os.File
	path  string
	force bool
	done  bool
}

// CreateAtomic creates a temporary file next to path.
func CreateAtomic(path string, force bool, perm os.FileMode) (*AtomicFile, error) {
	if !force {
		if _, err := os.Lstat(path); err == nil {
			return nil, fmt.Errorf("output file '%s' already exists. Use --force to overwrite", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}

	f, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create output: %w", err)
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("create output: %w", err)
	}

	return &AtomicFile{f: f, path: path, force: force}, nil
}

func (a *AtomicFile) Write(p []byte) (int, error) { return a.f.Write(p) }

// File exposes the underlying temporary file (e.g. for Chmod).
func (a *AtomicFile) File() *os.File { return a.f }

// Commit flushes the data to disk and moves it into place.
func (a *AtomicFile) Commit() error {
	if a.done {
		return errors.New("output already finalized")
	}
	a.done = true

	tmp := a.f.Name()
	if err := a.f.Sync(); err != nil {
		a.f.Close()
		os.Remove(tmp)
		return fmt.Errorf("sync output: %w", err)
	}
	if err := a.f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close output: %w", err)
	}

	if !a.force {
		// Link fails if the target appeared meanwhile, so we never clobber a
		// file created after the initial existence check.
		if err := os.Link(tmp, a.path); err == nil {
			os.Remove(tmp)
			return syncDir(a.path)
		} else if errors.Is(err, os.ErrExist) {
			os.Remove(tmp)
			return fmt.Errorf("output file '%s' already exists. Use --force to overwrite", a.path)
		}
		// Hard links unsupported (some filesystems): fall back to rename.
	}

	if err := os.Rename(tmp, a.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("finalize output: %w", err)
	}
	return syncDir(a.path)
}

// Abort discards the temporary file. It is safe to call after Commit.
func (a *AtomicFile) Abort() {
	if a.done {
		return
	}
	a.done = true
	a.f.Close()
	os.Remove(a.f.Name())
}

func syncDir(path string) error {
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil // best effort
	}
	defer d.Close()
	d.Sync() // not supported everywhere (e.g. Windows); ignore
	return nil
}
