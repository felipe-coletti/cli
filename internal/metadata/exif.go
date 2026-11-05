package metadata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// A minimal, bounds-checked TIFF/EXIF reader: just enough to tell the user
// what an EXIF block reveals (GPS, camera, dates, ...) and to read the
// orientation that should survive stripping.

var exifPrefix = []byte("Exif\x00\x00")

type tiffReader struct {
	b  []byte
	bo binary.ByteOrder
}

type tiffEntry struct {
	typ   uint16
	count uint32
	val   []byte
}

var errBadTIFF = errors.New("invalid EXIF data")

func newTIFF(b []byte) (*tiffReader, uint32, error) {
	b = bytes.TrimPrefix(b, exifPrefix)
	if len(b) < 8 {
		return nil, 0, errBadTIFF
	}
	t := &tiffReader{b: b}
	switch string(b[:4]) {
	case "II*\x00":
		t.bo = binary.LittleEndian
	case "MM\x00*":
		t.bo = binary.BigEndian
	default:
		return nil, 0, errBadTIFF
	}
	return t, t.bo.Uint32(b[4:8]), nil
}

var typeSize = map[uint16]uint32{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8}

// ifd reads the directory at off and returns its entries and the next IFD offset.
func (t *tiffReader) ifd(off uint32) (map[uint16]tiffEntry, uint32, error) {
	if off < 8 || uint64(off)+2 > uint64(len(t.b)) {
		return nil, 0, errBadTIFF
	}
	n := uint32(t.bo.Uint16(t.b[off:]))
	if n > 1000 || uint64(off)+2+uint64(n)*12+4 > uint64(len(t.b)) {
		return nil, 0, errBadTIFF
	}

	entries := make(map[uint16]tiffEntry, n)
	for i := range n {
		e := t.b[off+2+i*12 : off+2+(i+1)*12]
		tag, typ, count := t.bo.Uint16(e), t.bo.Uint16(e[2:]), t.bo.Uint32(e[4:])
		size, ok := typeSize[typ]
		if !ok || count > 1<<20 {
			continue
		}
		total := uint64(size) * uint64(count)
		var val []byte
		if total <= 4 {
			val = e[8 : 8+total]
		} else {
			vo := uint64(t.bo.Uint32(e[8:]))
			if vo+total > uint64(len(t.b)) {
				continue
			}
			val = t.b[vo : vo+total]
		}
		entries[tag] = tiffEntry{typ: typ, count: count, val: val}
	}

	next := t.bo.Uint32(t.b[off+2+n*12:])
	return entries, next, nil
}

func (t *tiffReader) str(e tiffEntry, ok bool) string {
	if !ok || e.typ != 2 {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(string(e.val), "\x00"))
}

func (t *tiffReader) uint(e tiffEntry, ok bool) uint32 {
	if !ok || len(e.val) == 0 {
		return 0
	}
	switch e.typ {
	case 3:
		return uint32(t.bo.Uint16(e.val))
	case 4:
		return t.bo.Uint32(e.val)
	}
	return 0
}

func (t *tiffReader) rationals(e tiffEntry, ok bool) []float64 {
	if !ok || e.typ != 5 {
		return nil
	}
	var out []float64
	for i := 0; i+8 <= len(e.val); i += 8 {
		num, den := t.bo.Uint32(e.val[i:]), t.bo.Uint32(e.val[i+4:])
		if den == 0 {
			return nil
		}
		out = append(out, float64(num)/float64(den))
	}
	return out
}

func (t *tiffReader) coord(e tiffEntry, ok bool, ref string) (float64, bool) {
	r := t.rationals(e, ok)
	if len(r) != 3 {
		return 0, false
	}
	v := r[0] + r[1]/60 + r[2]/3600
	if ref == "S" || ref == "W" {
		v = -v
	}
	return v, true
}

// exifInfo summarises an EXIF block.
type exifInfo struct {
	details     []string
	orientation uint16
	// onlyOrientation: the block holds nothing but the orientation tag
	// (e.g. it was already stripped with KeepOrientation).
	onlyOrientation bool
}

// item describes the EXIF block for the removed list.
func (e exifInfo) item() Item {
	if len(e.details) == 0 && e.orientation != 0 {
		return Item{"EXIF", "orientation only"}
	}
	return Item{"EXIF", summary(e.details)}
}

// removable reports whether the block must be reported as removed.
func (e exifInfo) removable(opts Options) bool {
	return !(opts.KeepOrientation && e.onlyOrientation)
}

func parseExif(b []byte) exifInfo {
	var info exifInfo

	t, off, err := newTIFF(b)
	if err != nil {
		return info
	}
	ifd0, next, err := t.ifd(off)
	if err != nil {
		return info
	}

	get := func(m map[uint16]tiffEntry, tag uint16) (tiffEntry, bool) {
		e, ok := m[tag]
		return e, ok
	}

	// GPS first: it is the most sensitive part.
	if gpsOff := t.uint(get(ifd0, 0x8825)); gpsOff != 0 {
		if gps, _, err := t.ifd(gpsOff); err == nil {
			lat, okLat := t.coord(gps[2], hasTag(gps, 2), t.str(get(gps, 1)))
			lon, okLon := t.coord(gps[4], hasTag(gps, 4), t.str(get(gps, 3)))
			if okLat && okLon {
				info.details = append(info.details, fmt.Sprintf("GPS %.6f, %.6f", lat, lon))
			} else if len(gps) > 0 {
				info.details = append(info.details, "GPS data")
			}
		}
	}

	camera := strings.TrimSpace(t.str(get(ifd0, 0x010F)) + " " + t.str(get(ifd0, 0x0110)))
	if camera != "" {
		info.details = append(info.details, "camera "+camera)
	}

	date := t.str(get(ifd0, 0x0132))
	if exifOff := t.uint(get(ifd0, 0x8769)); exifOff != 0 {
		if ex, _, err := t.ifd(exifOff); err == nil {
			if d := t.str(get(ex, 0x9003)); d != "" {
				date = d
			}
			if lens := t.str(get(ex, 0xA434)); lens != "" {
				info.details = append(info.details, "lens "+lens)
			}
			for _, tag := range []uint16{0xA431, 0xA435} {
				if s := t.str(get(ex, tag)); s != "" {
					info.details = append(info.details, "serial number "+s)
				}
			}
			if s := t.str(get(ex, 0xA430)); s != "" {
				info.details = append(info.details, "owner "+s)
			}
		}
	}
	if date != "" {
		info.details = append(info.details, "date "+date)
	}

	for _, f := range []struct {
		tag   uint16
		label string
	}{{0x013B, "artist"}, {0x8298, "copyright"}, {0x0131, "software"}, {0x010E, "description"}} {
		if s := t.str(get(ifd0, f.tag)); s != "" {
			info.details = append(info.details, f.label+" "+truncate(s, 40))
		}
	}

	if next != 0 {
		info.details = append(info.details, "thumbnail")
	}

	if o := t.uint(get(ifd0, 0x0112)); o >= 1 && o <= 8 {
		info.orientation = uint16(o)
		info.onlyOrientation = len(ifd0) == 1 && next == 0
	}

	return info
}

func hasTag(m map[uint16]tiffEntry, tag uint16) bool {
	_, ok := m[tag]
	return ok
}

// orientationTIFF builds the smallest valid TIFF block holding only the
// orientation tag, which carries no personal information.
func orientationTIFF(o uint16) []byte {
	b := make([]byte, 0, 26)
	b = append(b, "MM\x00*"...)
	b = binary.BigEndian.AppendUint32(b, 8)      // IFD0 offset
	b = binary.BigEndian.AppendUint16(b, 1)      // one entry
	b = binary.BigEndian.AppendUint16(b, 0x0112) // Orientation
	b = binary.BigEndian.AppendUint16(b, 3)      // SHORT
	b = binary.BigEndian.AppendUint32(b, 1)      // count
	b = binary.BigEndian.AppendUint16(b, o)
	b = append(b, 0, 0)                     // value padding
	b = binary.BigEndian.AppendUint32(b, 0) // no next IFD
	return b
}

func orientationLabel(o uint16) string {
	labels := map[uint16]string{
		2: "mirrored", 3: "rotated 180°", 4: "mirrored vertically", 5: "mirrored + rotated 90°",
		6: "rotated 90°", 7: "mirrored + rotated 270°", 8: "rotated 270°",
	}
	return "orientation (" + labels[o] + ")"
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
