package metadata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"
)

// xmpUUID identifies an XMP "uuid" box.
var xmpUUID = []byte{0xBE, 0x7A, 0xCF, 0xCB, 0x97, 0xA9, 0x42, 0xE8, 0x9C, 0x71, 0x99, 0x94, 0x91, 0xE3, 0xAF, 0xAC}

// Top-level atoms a QuickTime file may start with when it has no ftyp.
var qtFirstAtoms = map[string]bool{"moov": true, "mdat": true, "wide": true, "free": true, "skip": true, "pnot": true}

func isBMFF(b []byte) bool {
	return len(b) >= 8 && (string(b[4:8]) == "ftyp" || qtFirstAtoms[string(b[4:8])])
}

type bmffScan struct {
	*bmff
	res        *Result
	patches    []Patch
	recorded   time.Time // earliest creation time found
	timestamps bool
	fragmented bool
}

// stripBMFF analyses an ISO BMFF file and returns the patches that clean it.
func stripBMFF(r io.ReaderAt, size int64, opts Options) (*Result, error) {
	s := &bmffScan{bmff: &bmff{r: r, size: size}, res: &Result{Format: "MP4"}}

	top, err := s.children(0, size)
	if err != nil {
		return nil, err
	}

	heif := false
	if ftyp, ok := find(top, "ftyp"); ok {
		p, err := s.payload(ftyp)
		if err != nil {
			return nil, err
		}
		s.res.Format, heif = brandFormat(p)
	}

	for _, b := range top {
		switch {
		case b.typ == "moov":
			if err := s.moov(b); err != nil {
				return nil, err
			}
		case b.typ == "meta" && heif:
			if err := s.heif(b); err != nil {
				return nil, err
			}
		case b.typ == "meta":
			s.remove(b, Item{"Metadata", s.describeMeta(b)})
		case b.typ == "uuid" && bytes.Equal(b.uuid, xmpUUID):
			s.remove(b, Item{"XMP", humanSize(int(b.size))})
		case b.typ == "moof":
			s.fragmented = true
		}
	}

	if s.timestamps {
		detail := "creation/modification times"
		if !s.recorded.IsZero() {
			detail = "recorded " + s.recorded.UTC().Format("2006-01-02 15:04:05 UTC")
		}
		s.res.Removed = append(s.res.Removed, Item{"Timestamps", detail})
	}
	if s.fragmented {
		s.res.Kept = append(s.res.Kept, Item{"Not covered", "fragmented file: per-fragment metadata is not inspected"})
	}

	s.res.Patches = s.patches
	return s.res, nil
}

// brandFormat names the file from its ftyp brands and reports whether it is HEIF.
func brandFormat(ftyp []byte) (string, bool) {
	var brands []string
	for i := 0; i+4 <= len(ftyp); i += 4 {
		if i == 4 {
			continue // minor version
		}
		brands = append(brands, string(ftyp[i:i+4]))
	}
	if len(brands) == 0 {
		return "MP4", false
	}
	switch brands[0] {
	case "heic", "heix", "heim", "heis", "hevc", "hevx":
		return "HEIC", true
	case "avif", "avis":
		return "AVIF", true
	case "qt  ":
		return "MOV", false
	case "M4A ", "M4B ", "M4P ":
		return "M4A", false
	}
	for _, b := range brands {
		if b == "mif1" || b == "msf1" {
			return "HEIF", true
		}
	}
	return "MP4", false
}

func (s *bmffScan) remove(b bbox, it Item) {
	s.patches = append(s.patches, wipe(b)...)
	s.res.Removed = append(s.res.Removed, it)
}

func (s *bmffScan) moov(moov bbox) error {
	kids, err := s.children(moov.payload(), moov.end())
	if err != nil {
		return err
	}
	for _, b := range kids {
		switch {
		case b.typ == "mvhd":
			if err := s.times(b, true); err != nil {
				return err
			}
		case b.typ == "trak":
			if err := s.trak(b); err != nil {
				return err
			}
		case b.typ == "udta":
			s.remove(b, Item{"User data", s.describeUdta(b)})
		case b.typ == "meta":
			s.remove(b, Item{"Metadata", s.describeMeta(b)})
		case b.typ == "uuid" && bytes.Equal(b.uuid, xmpUUID):
			s.remove(b, Item{"XMP", humanSize(int(b.size))})
		}
	}
	return nil
}

func (s *bmffScan) trak(trak bbox) error {
	kids, err := s.children(trak.payload(), trak.end())
	if err != nil {
		return err
	}

	mdia, ok := find(kids, "mdia")
	var mdiaKids []bbox
	if ok {
		if mdiaKids, err = s.children(mdia.payload(), mdia.end()); err != nil {
			return err
		}
	}

	// Timed metadata tracks (iPhone "mebx", GoPro "gpmd" GPS telemetry, ...):
	// zero their samples and drop the whole track.
	if hdlr, ok := find(mdiaKids, "hdlr"); ok {
		p, err := s.payload(hdlr)
		if err != nil {
			return err
		}
		if len(p) >= 12 && string(p[8:12]) == "meta" {
			return s.metadataTrack(trak, mdiaKids)
		}
	}

	for _, b := range kids {
		switch b.typ {
		case "tkhd":
			if err := s.times(b, false); err != nil {
				return err
			}
		case "udta":
			s.remove(b, Item{"Track user data", s.describeUdta(b)})
		case "meta":
			s.remove(b, Item{"Track metadata", s.describeMeta(b)})
		}
	}
	if mdhd, ok := find(mdiaKids, "mdhd"); ok {
		return s.times(mdhd, false)
	}
	return nil
}

// times zeroes the creation/modification times of mvhd, tkhd or mdhd.
func (s *bmffScan) times(b bbox, movie bool) error {
	p, err := s.payload(b)
	if err != nil || len(p) < 4 {
		return errBadBMFF
	}
	n := 4
	if p[0] == 1 {
		n = 8
	}
	if len(p) < 4+2*n {
		return errBadBMFF
	}
	fields := p[4 : 4+2*n]
	if bytes.Equal(fields, make([]byte, 2*n)) {
		return nil
	}

	var created uint64
	if n == 4 {
		created = uint64(binary.BigEndian.Uint32(fields))
	} else {
		created = binary.BigEndian.Uint64(fields)
	}
	if movie && created != 0 && created < 1<<40 {
		s.recorded = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(created) * time.Second)
	}

	s.timestamps = true
	s.patches = append(s.patches, Patch{Off: b.payload() + 4, Len: int64(2 * n)})
	return nil
}

func (s *bmffScan) metadataTrack(trak bbox, mdiaKids []bbox) error {
	var stbl []bbox
	if minf, ok := find(mdiaKids, "minf"); ok {
		if minfKids, err := s.children(minf.payload(), minf.end()); err == nil {
			if st, ok := find(minfKids, "stbl"); ok {
				stbl, _ = s.children(st.payload(), st.end())
			}
		}
	}

	entry := "metadata"
	if stsd, ok := find(stbl, "stsd"); ok {
		if p, err := s.payload(stsd); err == nil && len(p) >= 16 {
			entry = typeName(string(p[12:16]))
		}
	}

	samples, total, err := s.sampleRanges(stbl)
	if err != nil {
		s.res.Kept = append(s.res.Kept, Item{"Not covered", fmt.Sprintf("timed metadata track (%s): %v", entry, err)})
		return nil
	}

	s.patches = append(s.patches, samples...)
	s.remove(trak, Item{"Metadata track", fmt.Sprintf("%s, %s of timed data (may include GPS)", entry, humanSize(int(total)))})
	return nil
}

// sampleRanges returns zero patches covering every sample of a track.
func (s *bmffScan) sampleRanges(stbl []bbox) ([]Patch, int64, error) {
	stsz, ok := find(stbl, "stsz")
	if !ok {
		return nil, 0, fmt.Errorf("no stsz sample table")
	}
	stsc, ok1 := find(stbl, "stsc")
	stco, ok2 := find(stbl, "stco")
	co64, ok3 := find(stbl, "co64")
	if !ok1 || (!ok2 && !ok3) {
		return nil, 0, fmt.Errorf("incomplete sample tables")
	}

	read := func(b bbox) *cursor {
		p, err := s.payload(b)
		if err != nil {
			return &cursor{bad: true}
		}
		c := &cursor{b: p}
		c.take(4) // version, flags
		return c
	}

	sz := read(stsz)
	fixed, count := sz.u32(), sz.u32()
	if count > 1<<24 || (fixed == 0 && int(count) > len(sz.b)/4) {
		return nil, 0, errBadBMFF
	}
	sizes := make([]uint32, count)
	for i := range sizes {
		if fixed != 0 {
			sizes[i] = fixed
		} else {
			sizes[i] = sz.u32()
		}
	}

	sc := read(stsc)
	type run struct{ first, perChunk uint32 }
	runs := make([]run, min(sc.u32(), 1<<20))
	for i := range runs {
		runs[i] = run{sc.u32(), sc.u32()}
		sc.u32() // sample description index
	}

	var chunks []int64
	if ok2 {
		c := read(stco)
		for range min(c.u32(), 1<<22) {
			chunks = append(chunks, int64(c.u32()))
		}
		if c.bad {
			return nil, 0, errBadBMFF
		}
	} else {
		c := read(co64)
		for range min(c.u32(), 1<<22) {
			chunks = append(chunks, int64(c.u64()))
		}
		if c.bad {
			return nil, 0, errBadBMFF
		}
	}
	if sz.bad || sc.bad || len(runs) == 0 {
		return nil, 0, errBadBMFF
	}

	var patches []Patch
	var total int64
	sample, r := 0, 0
	for ci, off := range chunks {
		for r+1 < len(runs) && int(runs[r+1].first) <= ci+1 {
			r++
		}
		var n int64
		for k := uint32(0); k < runs[r].perChunk && sample < len(sizes); k++ {
			n += int64(sizes[sample])
			sample++
		}
		if n == 0 {
			continue
		}
		if off < 0 || off+n > s.size {
			return nil, 0, errBadBMFF
		}
		patches = append(patches, Patch{Off: off, Len: n})
		total += n
	}
	return patches, total, nil
}

var udtaLabels = map[string]string{
	"©xyz": "location", "©mak": "make", "©mod": "model", "©swr": "software",
	"©day": "date", "©too": "encoder", "©nam": "title", "©ART": "artist",
	"©cmt": "comment", "©des": "description", "loci": "location",
}

// describeUdta summarises a QuickTime/MP4 user data box.
func (s *bmffScan) describeUdta(b bbox) string {
	kids, err := s.children(b.payload(), b.end())
	if err != nil {
		return humanSize(int(b.size))
	}
	var details []string
	for _, k := range kids {
		name := typeName(k.typ)
		switch {
		case k.typ == "meta":
			if d := s.describeMeta(k); d != "" {
				details = append(details, d)
			}
		case strings.HasPrefix(name, "©"):
			p, _ := s.read(k.payload(), min(k.end()-k.payload(), 256))
			label := udtaLabels[name]
			if label == "" {
				label = name
			}
			if len(p) >= 4 {
				n := int(binary.BigEndian.Uint16(p))
				if 4+n <= len(p) {
					details = append(details, label+" "+truncate(string(p[4:4+n]), 40))
					continue
				}
			}
			details = append(details, label)
		case k.typ == "loci":
			details = append(details, s.describeLoci(k))
		default:
			if l := udtaLabels[name]; l != "" {
				name = l
			}
			details = append(details, name)
		}
	}
	return summary(capDetails(details))
}

// describeLoci decodes a 3GPP location box: version/flags, language,
// name, role, then longitude, latitude and altitude as 16.16 fixed point.
func (s *bmffScan) describeLoci(b bbox) string {
	p, err := s.read(b.payload(), min(b.end()-b.payload(), 512))
	if err != nil {
		return "location"
	}
	c := &cursor{b: p}
	c.take(6) // version/flags, language
	c.cstring()
	c.u8() // role
	lon, lat := int32(c.u32()), int32(c.u32())
	if c.bad {
		return "location"
	}
	return fmt.Sprintf("location %.4f, %.4f", float64(lat)/65536, float64(lon)/65536)
}

// describeMeta summarises a "meta" box with keys/ilst (Apple QuickTime
// metadata: location, make, model, software, creation date, ...).
func (s *bmffScan) describeMeta(b bbox) string {
	start := b.payload()
	if first, err := s.read(start, 4); err == nil && bytes.Equal(first, []byte{0, 0, 0, 0}) {
		start += 4 // ISO full box (QuickTime "meta" has no version/flags)
	}
	kids, err := s.children(start, b.end())
	if err != nil {
		return humanSize(int(b.size))
	}

	var keys []string
	if kb, ok := find(kids, "keys"); ok {
		p, _ := s.payload(kb)
		c := &cursor{b: p}
		c.take(4)
		for range min(c.u32(), 1000) {
			n := int(c.u32())
			c.take(4) // namespace
			keys = append(keys, string(c.take(n-8)))
			if c.bad {
				break
			}
		}
	}

	var details []string
	if ilst, ok := find(kids, "ilst"); ok {
		items, _ := s.children(ilst.payload(), ilst.end())
		for _, it := range items {
			name := typeName(it.typ)
			if idx := binary.BigEndian.Uint32([]byte(it.typ)); idx >= 1 && int(idx) <= len(keys) {
				name = keys[idx-1]
			}
			name = strings.TrimPrefix(name, "com.apple.quicktime.")
			if l := udtaLabels[name]; l != "" {
				name = l
			}
			if name == "location.ISO6709" {
				name = "location"
			}

			value := ""
			if data, err := s.children(it.payload(), it.end()); err == nil {
				if d, ok := find(data, "data"); ok {
					p, _ := s.read(d.payload(), min(d.end()-d.payload(), 256))
					if len(p) >= 8 && binary.BigEndian.Uint32(p)&0xFFFFFF == 1 {
						value = truncate(string(p[8:]), 40)
					}
				}
			}
			details = append(details, strings.TrimSpace(name+" "+value))
		}
	}
	return summary(capDetails(details))
}

func capDetails(d []string) []string {
	if len(d) > 12 {
		return append(d[:12:12], fmt.Sprintf("+%d more", len(d)-12))
	}
	return d
}
