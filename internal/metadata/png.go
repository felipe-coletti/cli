package metadata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
)

var errBadPNG = errors.New("invalid or truncated PNG")

// Ancillary PNG chunks that affect how the image is displayed or animated;
// every other ancillary chunk is metadata and is dropped. Critical chunks
// (uppercase first letter: IHDR, PLTE, IDAT, IEND) are always kept.
var pngRenderChunks = map[string]bool{
	"tRNS": true, "gAMA": true, "cHRM": true, "sRGB": true, "iCCP": true,
	"sBIT": true, "bKGD": true, "pHYs": true, "sPLT": true, "hIST": true,
	"cICP": true, "mDCV": true, "cLLI": true, "mDCv": true, "cLLi": true,
	"acTL": true, "fcTL": true, "fdAT": true, // APNG animation
}

func stripPNG(b []byte, opts Options) (*Result, error) {
	res := &Result{Format: "PNG"}
	out := make([]byte, 0, len(b))
	out = append(out, pngSig...)

	var orientation uint16
	exifPos := -1
	i := len(pngSig)

	for {
		if i+8 > len(b) {
			return nil, errBadPNG
		}
		n := int(binary.BigEndian.Uint32(b[i:]))
		if n < 0 || n > len(b) || i+12+n > len(b) {
			return nil, errBadPNG
		}
		typ := string(b[i+4 : i+8])
		data := b[i+8 : i+8+n]
		chunk := b[i : i+12+n]
		i += 12 + n

		critical := typ[0] >= 'A' && typ[0] <= 'Z'

		switch {
		case critical || pngRenderChunks[typ]:
			if typ == "iCCP" {
				res.Kept = append(res.Kept, Item{"ICC profile", "color profile"})
			}
			out = append(out, chunk...)

		case typ == "tEXt" || typ == "zTXt" || typ == "iTXt":
			key, rest, _ := bytes.Cut(data, []byte{0})
			detail := string(key)
			if typ == "tEXt" && len(rest) > 0 {
				detail += ": " + truncate(string(rest), 50)
			}
			res.Removed = append(res.Removed, Item{"Text", detail})

		case typ == "eXIf":
			info := parseExif(data)
			if info.removable(opts) {
				res.Removed = append(res.Removed, info.item())
			}
			orientation = info.orientation
			exifPos = len(out)

		case typ == "tIME":
			res.Removed = append(res.Removed, Item{"Timestamp", "last modification time"})

		default:
			res.Removed = append(res.Removed, Item{"Chunk " + typ, humanSize(n)})
		}

		if typ == "IEND" {
			if rest := len(b) - i; rest > 0 {
				res.Removed = append(res.Removed, Item{"Trailing data", humanSize(rest) + " after the image"})
			}
			break
		}
	}

	if opts.KeepOrientation && orientation > 1 && exifPos >= 0 {
		tiff := orientationTIFF(orientation)
		c := binary.BigEndian.AppendUint32(nil, uint32(len(tiff)))
		c = append(c, "eXIf"...)
		c = append(c, tiff...)
		c = binary.BigEndian.AppendUint32(c, crc32.ChecksumIEEE(c[4:]))
		out = append(out[:exifPos:exifPos], append(c, out[exifPos:]...)...)
		res.Kept = append(res.Kept, Item{"EXIF", orientationLabel(orientation)})
	}

	res.Data = out
	return res, nil
}
