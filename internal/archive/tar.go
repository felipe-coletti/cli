package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func PackTar(input string, output io.Writer) error {
	_, err := os.Stat(input)
	if err != nil {
		return fmt.Errorf("stat input: %w", err)
	}

	tw := tar.NewWriter(output)
	defer tw.Close()

	return filepath.Walk(input, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(filepath.Dir(input), path)
		if err != nil {
			return fmt.Errorf("get relative path: %w", err)
		}

		relativePath = filepath.ToSlash(relativePath)

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("create tar header: %w", err)
		}

		header.Name = relativePath

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header: %w", err)
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer file.Close()

		if _, err := io.Copy(tw, file); err != nil {
			return fmt.Errorf("copy %s: %w", path, err)
		}

		return nil
	})
}

func ExtractTar(input io.Reader, output string, options ExtractOptions) error {
	tr := tar.NewReader(input)

	for {
		header, err := tr.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		target, err := securePath(output, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf(
					"create directory %s: %w",
					target,
					err,
				)
			}

		case tar.TypeReg:
			if err := extractFile(target, tr, header, options); err != nil {
				return err
			}

		default:
			return fmt.Errorf(
				"unsupported tar entry type %d: %s",
				header.Typeflag,
				header.Name,
			)
		}
	}

	return nil
}

func extractFile(target string, input io.Reader, header *tar.Header, options ExtractOptions) error {
	if _, err := os.Stat(target); err == nil && !options.Force {
		return fmt.Errorf(
			"file '%s' already exists. Use --force to overwrite",
			target,
		)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf(
			"create parent directory: %w",
			err,
		)
	}

	file, err := os.OpenFile(
		target,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		os.FileMode(header.Mode),
	)
	if err != nil {
		return fmt.Errorf(
			"create file %s: %w",
			target,
			err,
		)
	}

	if _, err := io.Copy(file, input); err != nil {
		file.Close()

		return fmt.Errorf(
			"extract %s: %w",
			target,
			err,
		)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf(
			"close %s: %w",
			target,
			err,
		)
	}

	return nil
}

func securePath(base, name string) (string, error) {
	name = filepath.Clean(filepath.FromSlash(name))

	if name == "." || name == "" {
		return base, nil
	}

	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve output path: %w", err)
	}

	target := filepath.Join(baseAbs, name)

	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve target path: %w", err)
	}

	relative, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("resolve relative path: %w", err)
	}

	if relative == ".." ||
		len(relative) >= 3 &&
			relative[:3] == ".."+string(os.PathSeparator) {
		return "", fmt.Errorf(
			"invalid archive path: %s",
			name,
		)
	}

	return targetAbs, nil
}
