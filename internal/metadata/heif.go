package metadata

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"
)

// HEIF/HEIC/AVIF keep metadata as *items* of the top-level meta box: an
// "Exif" item and an XMP item ("mime", application/rdf+xml), whose bytes
// live in mdat or idat at the locations given by iloc. Removing items would
// mean rewriting iinf/iloc/iref, so their bytes are overwritten in place
// with an empty but valid EXIF/XMP block of the same length instead. The
// display orientation of HEIF images comes from the irot/imir properties,
// which are untouched.

type heifItem struct {
	id          uint32
	typ, ctype  string
	method      uint16
	extents     [][2]int64 // absolute offset, length
	unsupported bool
}

func (s *bmffScan) heif(meta bbox) error {
	kids, err := s.children(meta.payload()+4, meta.end())
	if err != nil {
		return err
	}

	items := map[uint32]*heifItem{}
	if iinf, ok := find(kids, "iinf"); ok {
		if err := s.parseIINF(iinf, items); err != nil {
			return err
		}
	}
	idat, hasIdat := find(kids, "idat")
	if iloc, ok := find(kids, "iloc"); ok {
		if err := s.parseILOC(iloc, items, idat, hasIdat); err != nil {
			return err
		}
	}

	ids := make([]uint32, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	for _, id := range ids {
		it := items[id]
		isXMP := it.typ == "mime" && (strings.Contains(it.ctype, "rdf+xml") || strings.Contains(it.ctype, "xmp"))
		if it.typ != "Exif" && !isXMP {
			continue
		}
		if it.unsupported || len(it.extents) == 0 {
			s.res.Kept = append(s.res.Kept, Item{"Not covered", it.typ + " item stored in an unsupported way"})
			continue
		}

		data, err := s.itemData(it)
		if err != nil {
			return err
		}

		var repl []byte
		var item Item
		if isXMP {
			repl = emptyXMP(len(data))
			item = Item{"XMP", humanSize(len(data))}
		} else {
			repl = emptyHEIFExif(len(data))
			item = Item{"EXIF", heifExifSummary(data)}
		}
		if bytes.Equal(data, repl) {
			continue // already clean
		}

		s.res.Removed = append(s.res.Removed, item)
		pos := 0
		for _, e := range it.extents {
			s.patches = append(s.patches, Patch{Off: e[0], Len: e[1], Data: repl[pos : pos+int(e[1])]})
			pos += int(e[1])
		}
	}
	return nil
}

func (s *bmffScan) itemData(it *heifItem) ([]byte, error) {
	var data []byte
	for _, e := range it.extents {
		chunk, err := s.read(e[0], e[1])
		if err != nil {
			return nil, err
		}
		data = append(data, chunk...)
		if len(data) > maxBoxRead {
			return nil, errBadBMFF
		}
	}
	return data, nil
}

func (s *bmffScan) parseIINF(iinf bbox, items map[uint32]*heifItem) error {
	p, err := s.payload(iinf)
	if err != nil {
		return err
	}
	c := &cursor{b: p}
	version := c.u8()
	c.take(3)
	if version == 0 {
		c.u16()
	} else {
		c.u32()
	}
	if c.bad {
		return errBadBMFF
	}

	entries, err := s.children(iinf.payload()+int64(c.i), iinf.end())
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.typ != "infe" {
			continue
		}
		ep, err := s.payload(e)
		if err != nil {
			return err
		}
		ec := &cursor{b: ep}
		v := ec.u8()
		ec.take(3)
		if v < 2 {
			continue // legacy entries carry no item type
		}
		it := &heifItem{}
		if v == 2 {
			it.id = uint32(ec.u16())
		} else {
			it.id = ec.u32()
		}
		ec.u16() // protection index
		it.typ = string(ec.take(4))
		ec.cstring() // item name
		if it.typ == "mime" {
			it.ctype = strings.ToLower(ec.cstring())
		}
		if !ec.bad {
			items[it.id] = it
		}
	}
	return nil
}

func (s *bmffScan) parseILOC(iloc bbox, items map[uint32]*heifItem, idat bbox, hasIdat bool) error {
	p, err := s.payload(iloc)
	if err != nil {
		return err
	}
	c := &cursor{b: p}
	version := c.u8()
	c.take(3)
	sizes := c.u8()
	offSize, lenSize := int(sizes>>4), int(sizes&15)
	sizes = c.u8()
	baseSize, idxSize := int(sizes>>4), 0
	if version == 1 || version == 2 {
		idxSize = int(sizes & 15)
	}

	var count uint32
	if version < 2 {
		count = uint32(c.u16())
	} else {
		count = c.u32()
	}

	for range min(count, 1<<16) {
		var id uint32
		if version < 2 {
			id = uint32(c.u16())
		} else {
			id = c.u32()
		}
		var method uint16
		if version == 1 || version == 2 {
			method = c.u16() & 0xF
		}
		c.u16() // data reference index
		base := c.uN(baseSize)
		n := c.u16()

		var extents [][2]int64
		for range n {
			if idxSize > 0 {
				c.uN(idxSize)
			}
			off, length := c.uN(offSize), c.uN(lenSize)
			extents = append(extents, [2]int64{int64(base + off), int64(length)})
		}
		if c.bad {
			return errBadBMFF
		}

		it := items[id]
		if it == nil {
			continue
		}
		it.method = method
		switch {
		case method == 1 && hasIdat:
			for i := range extents {
				extents[i][0] += idat.payload()
			}
		case method != 0:
			it.unsupported = true
		}
		for _, e := range extents {
			if e[1] <= 0 || e[0] < 0 || e[0]+e[1] > s.size {
				it.unsupported = true // length 0 means "to the end": not for metadata
			}
		}
		it.extents = extents
	}
	return nil
}

// heifExifSummary parses an Exif item: a 4-byte offset to the TIFF header,
// then (usually) "Exif\0\0" and the TIFF data.
func heifExifSummary(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	off := int(binary.BigEndian.Uint32(data))
	if off < 0 || 4+off > len(data) {
		return ""
	}
	info := parseExif(data[4+off:])
	return info.item().Detail
}

// emptyHEIFExif is a valid Exif item of length n with an empty IFD.
func emptyHEIFExif(n int) []byte {
	b := make([]byte, n)
	min := []byte("\x00\x00\x00\x06Exif\x00\x00MM\x00*\x00\x00\x00\x08\x00\x00\x00\x00\x00\x00")
	if n >= len(min) {
		copy(b, min)
	}
	return b
}

// emptyXMP is a valid, empty XMP packet padded with spaces to length n.
func emptyXMP(n int) []byte {
	head := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/">` +
		`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"/></x:xmpmeta>`
	tail := `<?xpacket end="w"?>`
	b := bytes.Repeat([]byte(" "), n)
	if n >= len(head)+len(tail) {
		copy(b, head)
		copy(b[n-len(tail):], tail)
	}
	return b
}
