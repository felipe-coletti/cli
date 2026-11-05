package metadata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

var (
	xmpPrefix    = []byte("http://ns.adobe.com/xap/1.0/\x00")
	xmpExtPrefix = []byte("http://ns.adobe.com/xmp/extension/\x00")
	iccPrefix    = []byte("ICC_PROFILE\x00")
	errBadJPEG   = errors.New("invalid or truncated JPEG")
)

// JPEG rules. Kept: JFIF (APP0), ICC color profile (APP2), Adobe color
// transform (APP14) and everything needed to decode. Metadata: EXIF, XMP,
// IPTC/Photoshop, comments, vendor APPn blocks and any data after the end
// of the image (embedded previews, HDR gain maps, depth maps — which may
// carry their own EXIF).

type jpegSeg struct {
	m          byte
	start, end int    // from the 0xFF before the marker to the end of the segment
	payload    []byte // nil for standalone markers
}

// walkJPEG calls fn for every segment; for SOS the segment includes the
// entropy-coded data that follows. It returns the offset just past EOI.
func walkJPEG(b []byte, fn func(s jpegSeg)) (int, error) {
	i := 2
	for {
		if i >= len(b) || b[i] != 0xFF {
			return 0, errBadJPEG
		}
		for i < len(b) && b[i] == 0xFF { // fill bytes
			i++
		}
		if i >= len(b) {
			return 0, errBadJPEG
		}
		start, m := i-1, b[i]
		i++

		if m == 0xD9 { // EOI
			return i, nil
		}
		if (m >= 0xD0 && m <= 0xD7) || m == 0x01 { // standalone markers
			fn(jpegSeg{m: m, start: start, end: i})
			continue
		}

		if i+2 > len(b) {
			return 0, errBadJPEG
		}
		l := int(binary.BigEndian.Uint16(b[i:]))
		if l < 2 || i+l > len(b) {
			return 0, errBadJPEG
		}
		payload := b[i+2 : i+l]
		next := i + l

		if m == 0xDA { // SOS: entropy-coded data runs up to the next marker
			j := next
			for j+1 < len(b) {
				if b[j] == 0xFF {
					if n := b[j+1]; n != 0x00 && (n < 0xD0 || n > 0xD7) {
						break
					}
					j += 2
					continue
				}
				j++
			}
			if j+1 >= len(b) {
				return 0, errBadJPEG
			}
			next = j
		}

		fn(jpegSeg{m: m, start: start, end: next, payload: payload})
		i = next
	}
}

type jpegAction int

const (
	jpegKeep jpegAction = iota
	jpegKeepICC
	jpegRemove
)

// classifyJPEG decides what to do with a segment. For EXIF it also returns
// the parsed block (for the orientation).
func classifyJPEG(s jpegSeg, opts Options) (jpegAction, *Item, *exifInfo) {
	p := s.payload
	switch m := s.m; {
	case p == nil || m == 0xDA:
		return jpegKeep, nil, nil
	case m == 0xE0 && bytes.HasPrefix(p, []byte("JFIF\x00")),
		m == 0xEE && bytes.HasPrefix(p, []byte("Adobe")):
		return jpegKeep, nil, nil
	case m == 0xE2 && bytes.HasPrefix(p, iccPrefix):
		return jpegKeepICC, nil, nil
	case m == 0xE1 && bytes.HasPrefix(p, exifPrefix):
		info := parseExif(p)
		if !info.removable(opts) {
			return jpegRemove, nil, &info
		}
		it := info.item()
		return jpegRemove, &it, &info
	case m == 0xE1 && bytes.HasPrefix(p, xmpPrefix):
		return jpegRemove, &Item{"XMP", humanSize(len(p))}, nil
	case m == 0xE1 && bytes.HasPrefix(p, xmpExtPrefix):
		return jpegRemove, &Item{"XMP (extended)", humanSize(len(p))}, nil
	case m == 0xED:
		return jpegRemove, &Item{"IPTC/Photoshop", humanSize(len(p))}, nil
	case m == 0xFE:
		if isZero(p) {
			return jpegKeep, nil, nil // a comment already blanked by wipeJPEG
		}
		return jpegRemove, &Item{"Comment", truncate(string(p), 60)}, nil
	case m >= 0xE0 && m <= 0xEF: // other APPn: JFXX thumbnail, MPF index, Ducky, C2PA, ...
		return jpegRemove, &Item{fmt.Sprintf("APP%d", m-0xE0), appID(p)}, nil
	}
	return jpegKeep, nil, nil // DQT, DHT, SOFn, DRI, ...
}

// isBlank reports whether b holds only zeros and spaces (the padding
// written by wipeJPEG and by same-length re-encoding).
func isBlank(b []byte) bool {
	for _, c := range b {
		if c != 0 && c != ' ' {
			return false
		}
	}
	return true
}

func isZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

func stripJPEG(b []byte, opts Options) (*Result, error) {
	res := &Result{Format: "JPEG"}
	out := make([]byte, 0, len(b))
	out = append(out, 0xFF, 0xD8)

	var orientation uint16
	keptICC := false

	end, err := walkJPEG(b, func(s jpegSeg) {
		action, item, info := classifyJPEG(s, opts)
		if info != nil && info.orientation > 1 && orientation == 0 {
			orientation = info.orientation
		}
		switch action {
		case jpegKeepICC:
			if !keptICC {
				res.Kept = append(res.Kept, Item{"ICC profile", "color profile"})
				keptICC = true
			}
			fallthrough
		case jpegKeep:
			out = append(out, b[s.start:s.end]...)
		case jpegRemove:
			if item != nil {
				res.Removed = append(res.Removed, *item)
			}
		}
	})
	if err != nil {
		return nil, err
	}

	out = append(out, 0xFF, 0xD9)
	if rest := len(b) - end; rest > 0 {
		res.Removed = append(res.Removed, Item{"Trailing data", humanSize(rest) + " after the image (embedded previews, HDR or depth images)"})
	}

	if opts.KeepOrientation && orientation > 1 {
		tiff := orientationTIFF(orientation)
		seg := make([]byte, 0, 2+len(exifPrefix)+len(tiff))
		seg = binary.BigEndian.AppendUint16(seg, uint16(2+len(exifPrefix)+len(tiff)))
		seg = append(append(seg, exifPrefix...), tiff...)

		// Insert right after SOI (and APP0, if present).
		pos := 2
		if len(out) > 4 && out[2] == 0xFF && out[3] == 0xE0 {
			pos = 4 + int(binary.BigEndian.Uint16(out[4:]))
		}
		app1 := append([]byte{0xFF, 0xE1}, seg...)
		out = append(out[:pos:pos], append(app1, out[pos:]...)...)
		res.Kept = append(res.Kept, Item{"EXIF", orientationLabel(orientation)})
	}

	res.Data = out
	return res, nil
}

// wipeJPEG cleans a JPEG in place without changing its length, for images
// embedded in containers that address data by offset (PDF): every metadata
// segment becomes a zero-filled comment of the same size and data after
// the end of the image is zeroed. EXIF orientation is irrelevant there
// (PDF positions images with its own transform), so it is removed too.
func wipeJPEG(b []byte) ([]Item, error) {
	var items []Item
	type span struct{ start, end int }
	var wipe []span

	end, err := walkJPEG(b, func(s jpegSeg) {
		if action, item, _ := classifyJPEG(s, Options{}); action == jpegRemove {
			if item != nil {
				items = append(items, *item)
			}
			wipe = append(wipe, span{s.start, s.end})
		}
	})
	if err != nil {
		return nil, err
	}

	for _, w := range wipe {
		// 0xFF 0xFE <length> stays; the payload is zeroed.
		b[w.start+1] = 0xFE
		clear(b[w.start+4 : w.end])
	}
	if !isBlank(b[end:]) {
		items = append(items, Item{"Trailing data", humanSize(len(b) - end)})
		clear(b[end:])
	}
	return items, nil
}

// appID returns the printable identifier at the start of an APPn payload.
func appID(p []byte) string {
	end := bytes.IndexByte(p, 0)
	if end < 0 || end > 32 {
		end = min(len(p), 32)
	}
	id := p[:end]
	for _, c := range id {
		if c < 0x20 || c > 0x7E {
			return humanSize(len(p))
		}
	}
	if len(id) == 0 {
		return humanSize(len(p))
	}
	return string(id) + ", " + humanSize(len(p))
}
