package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func PackTar(input string, output io.Writer) error {
	if _, err := os.Lstat(input); err != nil {
		return fmt.Errorf("stat input: %w", err)
	}

	tw := tar.NewWriter(output)

	err := filepath.Walk(input, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(filepath.Dir(input), p)
		if err != nil {
			return fmt.Errorf("get relative path: %w", err)
		}

		// Walk uses Lstat, so symlinks are seen as links, never followed.
		link := ""
		switch mode := info.Mode(); {
		case mode.IsRegular(), mode.IsDir():
		case mode&os.ModeSymlink != 0:
			if link, err = os.Readlink(p); err != nil {
				return fmt.Errorf("read link %s: %w", p, err)
			}
		default:
			fmt.Fprintf(os.Stderr, "skip: %s (unsupported file type %s)\n", p, mode.Type())
			return nil
		}

		header, err := tar.FileInfoHeader(info, filepath.ToSlash(link))
		if err != nil {
			return fmt.Errorf("create tar header: %w", err)
		}
		header.Name = filepath.ToSlash(relativePath)

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header: %w", err)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		file, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open %s: %w", p, err)
		}
		defer file.Close()

		if _, err := io.Copy(tw, file); err != nil {
			return fmt.Errorf("copy %s: %w", p, err)
		}

		return nil
	})
	if err != nil {
		tw.Close()
		return err
	}

	// Close writes the end-of-archive marker; its error must not be dropped.
	if err := tw.Close(); err != nil {
		return fmt.Errorf("finalize tar: %w", err)
	}
	return nil
}

// ExtractTar extracts into output. Regular files and directories are
// written through an os.Root, so no entry can ever write outside output,
// even through symlinks created by earlier entries.
func ExtractTar(input io.Reader, output string, options ExtractOptions) error {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	root, err := os.OpenRoot(output)
	if err != nil {
		return fmt.Errorf("open output directory: %w", err)
	}
	defer root.Close()

	baseReal, err := filepath.EvalSymlinks(output)
	if err != nil {
		return fmt.Errorf("resolve output directory: %w", err)
	}

	tr := tar.NewReader(input)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		name, err := entryName(header.Name)
		if err != nil {
			return err
		}
		if name == "." {
			continue
		}
		perm := fs.FileMode(header.Mode).Perm()

		switch header.Typeflag {
		case tar.TypeDir:
			if err := mkdirAll(root, name, perm|0o700); err != nil {
				return fmt.Errorf("create directory %s: %w", name, err)
			}

		case tar.TypeReg:
			if err := extractFile(root, name, perm, tr, options); err != nil {
				return err
			}

		case tar.TypeSymlink:
			if err := extractSymlink(root, baseReal, name, header.Linkname, options); err != nil {
				return err
			}

		default:
			return fmt.Errorf("unsupported tar entry type %q: %s", header.Typeflag, header.Name)
		}
	}
}

// entryName validates an archive path and returns it as a clean, relative,
// OS-specific path.
func entryName(name string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || filepath.VolumeName(clean) != "" {
		return "", fmt.Errorf("invalid archive path: %s", name)
	}
	return filepath.FromSlash(clean), nil
}

func mkdirAll(root *os.Root, name string, perm fs.FileMode) error {
	if name == "." || name == "" {
		return nil
	}
	parts := strings.Split(name, string(filepath.Separator))
	for i := range parts {
		p := filepath.Join(parts[:i+1]...)
		if err := root.Mkdir(p, perm); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return nil
}

// prepare creates the parent of name and removes an existing entry when
// forced, so new files and links never write through an old symlink.
func prepare(root *os.Root, name string, options ExtractOptions) error {
	if err := mkdirAll(root, filepath.Dir(name), 0o755); err != nil {
		return fmt.Errorf("create parent directory of %s: %w", name, err)
	}

	if _, err := root.Lstat(name); err == nil {
		if !options.Force {
			return fmt.Errorf("file '%s' already exists. Use --force to overwrite", name)
		}
		if err := root.Remove(name); err != nil {
			return fmt.Errorf("replace %s: %w", name, err)
		}
	}
	return nil
}

func extractFile(root *os.Root, name string, perm fs.FileMode, input io.Reader, options ExtractOptions) error {
	if err := prepare(root, name, options); err != nil {
		return err
	}

	file, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("create file %s: %w", name, err)
	}

	if _, err := io.Copy(file, input); err != nil {
		file.Close()
		return fmt.Errorf("extract %s: %w", name, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	return nil
}

// extractSymlink creates a symlink only if its target stays inside the
// output directory. Targets must be relative and may only climb ("..") at
// the start; the number of levels climbed is checked against the real
// (symlink-resolved) depth of the link's directory.
func extractSymlink(root *os.Root, baseReal, name, target string, options ExtractOptions) error {
	invalid := fmt.Errorf("refusing symlink %s -> %s: target escapes the output directory", name, target)

	t := strings.ReplaceAll(target, `\`, "/")
	if t == "" || path.IsAbs(t) || filepath.VolumeName(t) != "" {
		return invalid
	}

	ups, climbing := 0, true
	for _, part := range strings.Split(t, "/") {
		switch {
		case part == "..":
			if !climbing {
				return invalid
			}
			ups++
		case part == "." || part == "":
		default:
			climbing = false
		}
	}

	if err := prepare(root, name, options); err != nil {
		return err
	}

	parentReal, err := filepath.EvalSymlinks(filepath.Join(baseReal, filepath.Dir(name)))
	if err != nil {
		return fmt.Errorf("resolve %s: %w", filepath.Dir(name), err)
	}
	rel, err := filepath.Rel(baseReal, parentReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return invalid
	}
	depth := 0
	if rel != "." {
		depth = len(strings.Split(rel, string(filepath.Separator)))
	}
	if ups > depth {
		return invalid
	}

	if err := os.Symlink(filepath.FromSlash(t), filepath.Join(parentReal, filepath.Base(name))); err != nil {
		return fmt.Errorf("create symlink %s: %w", name, err)
	}
	return nil
}
