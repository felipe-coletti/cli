package metadata

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"cli/internal/metadata"
)

// maxFileSize bounds how much is read into memory for formats rewritten
// in memory (photos, documents). MP4/MOV/HEIC are processed in place and
// have no limit.
const maxFileSize = 1 << 30

// analysis is a supported file and what cleaning it involves.
type analysis struct {
	f   *os.File
	fi  fs.FileInfo
	res *metadata.Result
}

func (a *analysis) Close() { a.f.Close() }

// analyze inspects path. Unsupported files (detected from their first
// bytes) return ErrUnsupported together with their FileInfo.
func analyze(path string, opts metadata.Options) (*analysis, fs.FileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	a := &analysis{f: f}
	ok := false
	defer func() {
		if !ok {
			f.Close()
		}
	}()

	if a.fi, err = f.Stat(); err != nil {
		return nil, nil, err
	}
	if !a.fi.Mode().IsRegular() {
		return nil, a.fi, metadata.ErrUnsupported
	}

	head := make([]byte, metadata.HeadSize)
	n, _ := io.ReadFull(f, head)
	format := metadata.Detect(head[:n])

	switch {
	case format == "":
		return nil, a.fi, metadata.ErrUnsupported
	case metadata.Streamed(format):
		a.res, err = metadata.StripAt(f, a.fi.Size(), opts)
	case a.fi.Size() > maxFileSize:
		return nil, a.fi, fmt.Errorf("file too large (%d bytes)", a.fi.Size())
	default:
		var data []byte
		if data, err = io.ReadAll(io.NewSectionReader(f, 0, a.fi.Size())); err == nil {
			a.res, err = metadata.Strip(data, opts)
		}
	}
	if err != nil {
		return nil, a.fi, err
	}

	ok = true
	return a, a.fi, nil
}

// collect expands directories into the regular files they contain.
func collect(paths []string) ([]string, error) {
	var files []string
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			files = append(files, p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func isUnsupported(err error) bool { return errors.Is(err, metadata.ErrUnsupported) }

func kindList(items []metadata.Item) string {
	seen := map[string]bool{}
	var kinds []string
	for _, it := range items {
		if !seen[it.Kind] {
			seen[it.Kind] = true
			kinds = append(kinds, it.Kind)
		}
	}
	return strings.Join(kinds, ", ")
}

const formatsHelp = `Supported: JPEG, PNG, WebP, HEIC/AVIF, MP4/MOV/M4A, PDF and DOCX/XLSX/PPTX.
Image, audio and video data is never re-encoded. Other files are reported
as unsupported.`
