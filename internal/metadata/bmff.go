package metadata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// ISO Base Media File Format (MP4, MOV, HEIC, AVIF, M4A, 3GP) support.
//
// These files can be many gigabytes and their media data is addressed by
// absolute file offsets, so nothing is ever moved: metadata boxes are
// turned into "free" boxes (which every reader skips) with their content
// zeroed, metadata items and timed-metadata samples are overwritten in
// place, and timestamps are zeroed. The file keeps its exact size and
// layout, and only box headers and small tables are read into memory.

var errBadBMFF = errors.New("invalid or truncated MP4/MOV/HEIF file")

// Patch overwrites Len bytes at Off with Data, or with zeros if Data is nil.
type Patch struct {
	Off, Len int64
	Data     []byte
}

type bbox struct {
	typ         string
	start, size int64
	base        int64 // size+type header length (8, or 16 with a 64-bit size)
	hdr         int64 // full header length (base + 16 for "uuid" boxes)
	uuid        []byte
}

func (b bbox) payload() int64 { return b.start + b.hdr }
func (b bbox) end() int64     { return b.start + b.size }

type bmff struct {
	r    io.ReaderAt
	size int64
}

const maxBoxRead = 64 << 20 // tables and metadata items read into memory

func (m *bmff) read(off, n int64) ([]byte, error) {
	if off < 0 || n < 0 || n > maxBoxRead || off+n > m.size {
		return nil, errBadBMFF
	}
	buf := make([]byte, n)
	if _, err := m.r.ReadAt(buf, off); err != nil {
		return nil, errBadBMFF
	}
	return buf, nil
}

// children lists the boxes in [start, end).
func (m *bmff) children(start, end int64) ([]bbox, error) {
	var out []bbox
	for off := start; end-off >= 8; {
		h, err := m.read(off, 8)
		if err != nil {
			return nil, err
		}
		b := bbox{typ: string(h[4:8]), start: off, base: 8}
		switch sz := binary.BigEndian.Uint32(h); sz {
		case 0: // extends to the end of the container
			b.size = end - off
		case 1: // 64-bit size follows
			ext, err := m.read(off+8, 8)
			if err != nil {
				return nil, err
			}
			b.size, b.base = int64(binary.BigEndian.Uint64(ext)), 16
		default:
			b.size = int64(sz)
		}
		b.hdr = b.base
		if b.typ == "uuid" {
			if b.uuid, err = m.read(off+b.base, 16); err != nil {
				return nil, err
			}
			b.hdr += 16
		}
		if b.size < b.hdr || b.size > end-off {
			return nil, errBadBMFF
		}
		out = append(out, b)
		if len(out) > 1<<16 {
			return nil, errBadBMFF
		}
		off += b.size
	}
	return out, nil
}

func (m *bmff) payload(b bbox) ([]byte, error) { return m.read(b.payload(), b.end()-b.payload()) }

func find(boxes []bbox, typ string) (bbox, bool) {
	for _, b := range boxes {
		if b.typ == typ {
			return b, true
		}
	}
	return bbox{}, false
}

// wipe turns b into a zero-filled "free" box of the same size.
func wipe(b bbox) []Patch {
	return []Patch{
		{Off: b.start + 4, Len: 4, Data: []byte("free")},
		{Off: b.start + b.base, Len: b.size - b.base},
	}
}

// typeName makes a box type printable ("\xa9xyz" -> "©xyz").
func typeName(t string) string {
	if strings.HasPrefix(t, "\xa9") {
		return "©" + t[1:]
	}
	for _, c := range []byte(t) {
		if c < 0x20 || c > 0x7E {
			return fmt.Sprintf("%x", t)
		}
	}
	return strings.TrimSpace(t)
}

// cursor reads big-endian fields from a byte slice, recording overruns.
type cursor struct {
	b   []byte
	i   int
	bad bool
}

func (c *cursor) take(n int) []byte {
	if c.bad || n < 0 || c.i+n > len(c.b) {
		c.bad = true
		// Fixed-size readers (u8..u64) need n bytes to index into; never
		// allocate a hostile length.
		return make([]byte, min(max(n, 0), 8))
	}
	v := c.b[c.i : c.i+n]
	c.i += n
	return v
}

func (c *cursor) u8() uint8   { return c.take(1)[0] }
func (c *cursor) u16() uint16 { return binary.BigEndian.Uint16(c.take(2)) }
func (c *cursor) u32() uint32 { return binary.BigEndian.Uint32(c.take(4)) }
func (c *cursor) u64() uint64 { return binary.BigEndian.Uint64(c.take(8)) }

// uN reads an unsigned integer of 0, 4 or 8 bytes (as used by iloc).
func (c *cursor) uN(n int) uint64 {
	switch n {
	case 0:
		return 0
	case 4:
		return uint64(c.u32())
	case 8:
		return c.u64()
	}
	c.bad = true
	return 0
}

func (c *cursor) cstring() string {
	if c.bad {
		return ""
	}
	end := bytes.IndexByte(c.b[c.i:], 0)
	if end < 0 {
		s := string(c.b[c.i:])
		c.i = len(c.b)
		return s
	}
	s := string(c.b[c.i : c.i+end])
	c.i += end + 1
	return s
}

// ApplyPatches copies src (of the given size) to dst with patches applied.
func ApplyPatches(dst io.Writer, src io.ReaderAt, size int64, patches []Patch) error {
	ps := slices.Clone(patches)
	slices.SortFunc(ps, func(a, b Patch) int { return int(a.Off - b.Off) })

	pos := int64(0)
	for _, p := range ps {
		if p.Off < pos || p.Off+p.Len > size {
			return errors.New("internal error: overlapping or out-of-range patch")
		}
		if _, err := io.Copy(dst, io.NewSectionReader(src, pos, p.Off-pos)); err != nil {
			return err
		}
		if err := writePatch(dst, p); err != nil {
			return err
		}
		pos = p.Off + p.Len
	}
	_, err := io.Copy(dst, io.NewSectionReader(src, pos, size-pos))
	return err
}

func writePatch(w io.Writer, p Patch) error {
	if p.Data != nil {
		_, err := w.Write(p.Data)
		return err
	}
	zeros := make([]byte, min(p.Len, 1<<16))
	for left := p.Len; left > 0; {
		n := min(left, int64(len(zeros)))
		if _, err := w.Write(zeros[:n]); err != nil {
			return err
		}
		left -= n
	}
	return nil
}

// PatchInPlace applies patches to an open file. Box-type changes are
// written first, so an interruption leaves a valid file with "free" boxes
// that still hold their old bytes, never a structurally broken one.
func PatchInPlace(f io.WriterAt, patches []Patch) error {
	ps := slices.Clone(patches)
	slices.SortStableFunc(ps, func(a, b Patch) int {
		ta, tb := a.Len == 4 && string(a.Data) == "free", b.Len == 4 && string(b.Data) == "free"
		switch {
		case ta && !tb:
			return -1
		case tb && !ta:
			return 1
		}
		return 0
	})
	for _, p := range ps {
		if err := writePatch(&offsetWriter{f, p.Off}, p); err != nil {
			return err
		}
	}
	return nil
}

type offsetWriter struct {
	w   io.WriterAt
	off int64
}

func (o *offsetWriter) Write(p []byte) (int, error) {
	n, err := o.w.WriteAt(p, o.off)
	o.off += int64(n)
	return n, err
}

func applyToBytes(b []byte, patches []Patch) []byte {
	out := slices.Clone(b)
	for _, p := range patches {
		if p.Data != nil {
			copy(out[p.Off:], p.Data)
		} else {
			clear(out[p.Off : p.Off+p.Len])
		}
	}
	return out
}
