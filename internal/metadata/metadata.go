// Package metadata finds and removes metadata (EXIF, GPS, XMP, document
// properties, ...) from files without re-encoding their content.
//
// Supported formats: JPEG, PNG, WebP, HEIC/HEIF/AVIF, MP4/MOV/M4A, PDF and
// Office Open XML documents (DOCX, XLSX, PPTX). Media data is never
// re-encoded: metadata blocks are dropped (or, in MP4/HEIF, overwritten in
// place) and the image/audio/video data is kept byte for byte.
package metadata

import (
	"bytes"
	"errors"
	"io"
	"strings"
)

// ErrUnsupported is returned for files whose format is not handled.
var ErrUnsupported = errors.New("unsupported file format")

// Options controls what is removed.
type Options struct {
	// KeepOrientation preserves the EXIF orientation tag, without which many
	// phone photos are displayed rotated. It reveals nothing personal.
	KeepOrientation bool
}

// Item describes one piece of metadata.
type Item struct {
	Kind   string // e.g. "EXIF", "XMP", "Comment"
	Detail string // human-readable summary, may be empty
}

// Result is the outcome of Strip.
type Result struct {
	Format string
	// Data is the cleaned file, for formats rewritten in memory. For
	// streamed formats (see Streamed) Data is nil when produced by StripAt
	// and the cleaning is described by Patches instead.
	Data    []byte
	Patches []Patch
	Removed []Item // metadata that was removed
	Kept    []Item // notable metadata that was intentionally kept or not covered
}

// Changed reports whether anything was removed.
func (r *Result) Changed() bool { return len(r.Removed) > 0 }

type format struct {
	name   string
	detect func(head []byte) bool
	strip  func(data []byte, opts Options) (*Result, error)
	// stream, when set, cleans the file through a ReaderAt (large files).
	stream func(r io.ReaderAt, size int64, opts Options) (*Result, error)
}

var formats = []format{
	{name: "JPEG", detect: isJPEG, strip: stripJPEG},
	{name: "PNG", detect: isPNG, strip: stripPNG},
	{name: "WebP", detect: isWebP, strip: stripWebP},
	{name: "Office", detect: isZip, strip: stripOOXML},
	{name: "PDF", detect: isPDF, strip: stripPDF},
	{name: "BMFF", detect: isBMFF, stream: stripBMFF},
}

// Streamed reports whether a format (as returned by Detect) is processed
// with StripAt, without reading the whole file into memory.
func Streamed(name string) bool { return name == "BMFF" }

// StripAt analyses a streamed-format file and returns the patches that
// clean it (apply them with ApplyPatches or PatchInPlace).
func StripAt(r io.ReaderAt, size int64, opts Options) (*Result, error) {
	head := make([]byte, HeadSize)
	n, _ := r.ReadAt(head, 0)
	for _, f := range formats {
		if f.stream != nil && f.detect(head[:n]) {
			return f.stream(r, size, opts)
		}
	}
	return nil, ErrUnsupported
}

// HeadSize is how many leading bytes Detect needs.
const HeadSize = 16

// Detect reports the probable format from the first HeadSize bytes, or ""
// if the file is certainly unsupported (so callers can skip reading it).
func Detect(head []byte) string {
	for _, f := range formats {
		if f.detect(head) {
			return f.name
		}
	}
	return ""
}

// Strip removes metadata from data. The input slice is not modified.
// Calling it without using Result.Data is how metadata is listed: the
// listing is exactly what stripping would remove.
func Strip(data []byte, opts Options) (*Result, error) {
	for _, f := range formats {
		if !f.detect(data) {
			continue
		}
		if f.strip != nil {
			return f.strip(data, opts)
		}
		res, err := f.stream(bytes.NewReader(data), int64(len(data)), opts)
		if err != nil {
			return nil, err
		}
		res.Data = applyToBytes(data, res.Patches)
		return res, nil
	}
	return nil, ErrUnsupported
}

func isJPEG(b []byte) bool { return len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF }

var pngSig = []byte("\x89PNG\r\n\x1a\n")

func isPNG(b []byte) bool { return bytes.HasPrefix(b, pngSig) }

func isWebP(b []byte) bool {
	return len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
}

func isZip(b []byte) bool { return bytes.HasPrefix(b, []byte("PK\x03\x04")) }

func summary(details []string) string { return strings.Join(details, " · ") }
