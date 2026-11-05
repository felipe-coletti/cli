package metadata

import (
	"encoding/binary"
	"errors"
)

var errBadWebP = errors.New("invalid or truncated WebP")

const (
	vp8xFlagXMP  = 0x04
	vp8xFlagEXIF = 0x08
)

// stripWebP drops the EXIF and XMP chunks of a RIFF/WebP file and updates
// the VP8X feature flags and the RIFF size accordingly.
func stripWebP(b []byte, opts Options) (*Result, error) {
	res := &Result{Format: "WebP"}

	riffSize := int(binary.LittleEndian.Uint32(b[4:8]))
	end := min(len(b), 8+riffSize)
	if end < 12 {
		return nil, errBadWebP
	}
	if rest := len(b) - end; rest > 0 {
		res.Removed = append(res.Removed, Item{"Trailing data", humanSize(rest) + " after the image"})
	}

	out := make([]byte, 0, len(b))
	out = append(out, b[:12]...)

	var orientation uint16
	exifPos, vp8xPos := -1, -1

	for i := 12; i < end; {
		if i+8 > end {
			return nil, errBadWebP
		}
		id := string(b[i : i+4])
		n := int(binary.LittleEndian.Uint32(b[i+4:]))
		padded := n + n%2
		if n < 0 || i+8+n > end {
			return nil, errBadWebP
		}
		data := b[i+8 : i+8+n]
		chunk := b[i:min(end, i+8+padded)]
		i += 8 + padded

		switch id {
		case "EXIF":
			info := parseExif(data)
			if info.removable(opts) {
				res.Removed = append(res.Removed, info.item())
			}
			orientation = info.orientation
			exifPos = len(out)
		case "XMP ":
			res.Removed = append(res.Removed, Item{"XMP", humanSize(n)})
		default:
			if id == "VP8X" {
				vp8xPos = len(out)
			}
			if id == "ICCP" {
				res.Kept = append(res.Kept, Item{"ICC profile", "color profile"})
			}
			out = append(out, chunk...)
		}
	}

	keepOrientation := opts.KeepOrientation && orientation > 1 && exifPos >= 0 && vp8xPos >= 0
	if keepOrientation {
		tiff := orientationTIFF(orientation)
		c := append([]byte("EXIF"), binary.LittleEndian.AppendUint32(nil, uint32(len(tiff)))...)
		c = append(c, tiff...) // 26 bytes: even, no padding needed
		out = append(out[:exifPos:exifPos], append(c, out[exifPos:]...)...)
		res.Kept = append(res.Kept, Item{"EXIF", orientationLabel(orientation)})
	}

	if vp8xPos >= 0 && vp8xPos+9 <= len(out) {
		flags := out[vp8xPos+8] &^ (vp8xFlagXMP | vp8xFlagEXIF)
		if keepOrientation {
			flags |= vp8xFlagEXIF
		}
		out[vp8xPos+8] = flags
	}

	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	res.Data = out
	return res, nil
}
