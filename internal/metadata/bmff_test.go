package metadata

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func box(typ string, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	b := binary.BigEndian.AppendUint32(nil, uint32(8+len(body)))
	return append(append(b, typ...), body...)
}

func u32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func u16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }

// fullTimes is a version-0 mvhd/tkhd/mdhd prefix with creation and modification times.
func fullTimes(created uint32, rest int) []byte {
	b := append(u32(0), u32(created)...)
	b = append(b, u32(created)...)
	return append(b, make([]byte, rest)...)
}

func qtText(s string) []byte { return append(append(u16(uint16(len(s))), u16(0x55c4)...), s...) }

func dataBox(s string) []byte { return box("data", u32(1), u32(0), []byte(s)) }

// buildMP4 returns an iPhone-like movie and the offsets of its media data.
func buildMP4(t *testing.T) (file []byte, videoAt, gpsAt int) {
	t.Helper()
	const created = 3797403721 // 2024-05-01 10:22:01 UTC in seconds since 1904

	video := []byte("VIDEO-FRAMES-THAT-MUST-SURVIVE")
	gps := []byte("GPSGPS+37.7749-122.4194")

	keys := box("keys", u32(0), u32(2),
		u32(uint32(8+len("com.apple.quicktime.location.ISO6709"))), []byte("mdta"), []byte("com.apple.quicktime.location.ISO6709"),
		u32(uint32(8+len("com.apple.quicktime.make"))), []byte("mdta"), []byte("com.apple.quicktime.make"))
	ilst := box("ilst",
		box("\x00\x00\x00\x01", dataBox("+37.7749-122.4194+010.000/")),
		box("\x00\x00\x00\x02", dataBox("Apple")))
	qtMeta := box("meta", box("hdlr", u32(0), u32(0), []byte("mdta"), make([]byte, 13)), keys, ilst)

	udta := box("udta", box("\xa9xyz", qtText("+37.7749-122.4194/")), box("\xa9mod", qtText("iPhone 13")))

	videoTrak := box("trak",
		box("tkhd", fullTimes(created, 80)),
		box("mdia", box("mdhd", fullTimes(created, 8)), box("hdlr", u32(0), u32(0), []byte("vide"), make([]byte, 13))))

	// The metadata track's chunk offset is patched once the layout is known.
	stco := box("stco", u32(0), u32(1), u32(0))
	metaTrak := box("trak",
		box("tkhd", fullTimes(created, 80)),
		box("mdia",
			box("mdhd", fullTimes(created, 8)),
			box("hdlr", u32(0), u32(0), []byte("meta"), make([]byte, 13)),
			box("minf", box("stbl",
				box("stsd", u32(0), u32(1), box("mebx", make([]byte, 8))),
				box("stsz", u32(0), u32(0), u32(2), u32(6), u32(uint32(len(gps)-6))),
				box("stsc", u32(0), u32(1), u32(1), u32(2), u32(1)),
				stco))))

	ftyp := box("ftyp", []byte("qt  "), u32(0), []byte("qt  "))
	moov := box("moov", box("mvhd", fullTimes(created, 88)), videoTrak, metaTrak, udta, qtMeta)
	mdat := box("mdat", video, gps)

	file = append(append(ftyp, moov...), mdat...)
	videoAt = len(ftyp) + len(moov) + 8
	gpsAt = videoAt + len(video)

	// Patch the stco entry (last 4 bytes of the stco box inside moov).
	stcoAt := len(ftyp) + bytes.Index(moov, stco) + len(stco) - 4
	binary.BigEndian.PutUint32(file[stcoAt:], uint32(gpsAt))
	return file, videoAt, gpsAt
}

func TestMP4(t *testing.T) {
	file, videoAt, _ := buildMP4(t)

	res, err := Strip(file, Options{KeepOrientation: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "MOV" {
		t.Errorf("format %s", res.Format)
	}
	got := map[string]string{}
	for _, it := range res.Removed {
		got[it.Kind] = it.Detail
	}
	for kind, want := range map[string]string{
		"Metadata track": "mebx",
		"User data":      "location +37.7749-122.4194/ · model iPhone 13",
		"Metadata":       "location +37.7749-122.4194+010.000/ · make Apple",
		"Timestamps":     "recorded 2024-05-01 10:22:01 UTC",
	} {
		if !strings.Contains(got[kind], want) {
			t.Errorf("%s: got %q, want %q", kind, got[kind], want)
		}
	}

	if len(res.Data) != len(file) {
		t.Fatal("file size must not change")
	}
	for _, leak := range []string{"Apple", "iPhone", "+37.77", "GPSGPS"} {
		if bytes.Contains(res.Data, []byte(leak)) {
			t.Errorf("cleaned file still contains %q", leak)
		}
	}
	if !bytes.Equal(res.Data[videoAt:videoAt+30], file[videoAt:videoAt+30]) {
		t.Fatal("video data changed")
	}

	again, err := Strip(res.Data, Options{})
	if err != nil {
		t.Fatalf("cleaned file no longer parses: %v", err)
	}
	if again.Changed() {
		t.Fatalf("second pass found: %+v", again.Removed)
	}

	// Streaming and in-place paths produce the same bytes as in memory.
	at, _ := StripAt(bytes.NewReader(file), int64(len(file)), Options{})
	var streamed bytes.Buffer
	if err := ApplyPatches(&streamed, bytes.NewReader(file), int64(len(file)), at.Patches); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(streamed.Bytes(), res.Data) {
		t.Fatal("ApplyPatches differs from Strip")
	}
	inPlace := &memFile{b: bytes.Clone(file)}
	if err := PatchInPlace(inPlace, at.Patches); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(inPlace.b, res.Data) {
		t.Fatal("PatchInPlace differs from Strip")
	}
}

type memFile struct{ b []byte }

func (m *memFile) WriteAt(p []byte, off int64) (int, error) { return copy(m.b[off:], p), nil }

func buildHEIC(t *testing.T) (file []byte, imageAt int) {
	t.Helper()
	image := []byte("HEVC-IMAGE-DATA-MUST-SURVIVE")
	exif := append(append(u32(6), exifPrefix...), sampleTIFF()...)
	xmp := []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF>Fulano</rdf:RDF></x:xmpmeta>`)

	infe := func(id uint16, typ string, extra ...byte) []byte {
		return box("infe", []byte{2, 0, 0, 0}, u16(id), u16(0), []byte(typ), []byte{0}, extra)
	}
	iinf := box("iinf", u32(0), u16(3),
		infe(1, "hvc1"),
		infe(2, "Exif"),
		infe(3, "mime", append([]byte("application/rdf+xml"), 0)...))

	// iloc v0, offset_size=4, length_size=4, base_offset_size=0.
	ilocBody := func(offs [3]uint32) []byte {
		b := append(u32(0), 0x44, 0x00)
		b = append(b, u16(3)...)
		lens := [3]int{len(image), len(exif), len(xmp)}
		for i := range 3 {
			b = append(b, u16(uint16(i+1))...)
			b = append(b, u16(0)...) // data reference index
			b = append(b, u16(1)...) // one extent
			b = append(b, u32(offs[i])...)
			b = append(b, u32(uint32(lens[i]))...)
		}
		return b
	}

	ftyp := box("ftyp", []byte("heic"), u32(0), []byte("mif1heic"))
	build := func(offs [3]uint32) []byte {
		meta := box("meta", u32(0), box("hdlr", u32(0), u32(0), []byte("pict"), make([]byte, 13)),
			box("pitm", u32(0), u16(1)), iinf, box("iloc", ilocBody(offs)))
		return append(append(ftyp, meta...), box("mdat", image, exif, xmp)...)
	}

	// Lay out once to learn the offsets, then build for real.
	draft := build([3]uint32{})
	mdatData := len(draft) - (len(image) + len(exif) + len(xmp))
	offs := [3]uint32{uint32(mdatData), uint32(mdatData + len(image)), uint32(mdatData + len(image) + len(exif))}
	return build(offs), mdatData
}

func TestHEIC(t *testing.T) {
	file, imageAt := buildHEIC(t)

	res, err := Strip(file, Options{KeepOrientation: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "HEIC" || kinds(res.Removed) != "EXIF,XMP" {
		t.Fatalf("format %s, removed %s", res.Format, kinds(res.Removed))
	}
	if !strings.Contains(res.Removed[0].Detail, "GPS -23.550000, -46.633333") {
		t.Fatalf("EXIF summary: %q", res.Removed[0].Detail)
	}
	for _, leak := range []string{"Apple", "iPhone", "Fulano"} {
		if bytes.Contains(res.Data, []byte(leak)) {
			t.Errorf("cleaned file still contains %q", leak)
		}
	}
	if !bytes.Equal(res.Data[imageAt:imageAt+28], file[imageAt:imageAt+28]) {
		t.Fatal("image data changed")
	}
	if len(res.Data) != len(file) {
		t.Fatal("size changed")
	}

	again, err := Strip(res.Data, Options{})
	if err != nil || again.Changed() {
		t.Fatalf("second pass: %v %+v", err, again)
	}
}

func TestBMFFGarbage(t *testing.T) {
	file, _, _ := buildMP4(t)
	heic, _ := buildHEIC(t)
	for _, b := range [][]byte{file[:40], heic[:len(heic)/2], []byte("\x00\x00\x00\x08ftyp"), []byte("\xff\xff\xff\xffmoov")} {
		Strip(b, Options{}) // must not panic
	}
}
