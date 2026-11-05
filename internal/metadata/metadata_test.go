package metadata

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"
)

// --- test fixtures --------------------------------------------------------

type tag struct {
	id    uint16
	typ   uint16
	count uint32
	data  []byte // raw value bytes (big endian)
}

func ascii(id uint16, s string) tag {
	return tag{id, 2, uint32(len(s) + 1), append([]byte(s), 0)}
}

func short(id uint16, v uint16) tag {
	return tag{id, 3, 1, binary.BigEndian.AppendUint16(nil, v)}
}

func long(id uint16, v uint32) tag { return tag{id, 4, 1, binary.BigEndian.AppendUint32(nil, v)} }

func rationals(id uint16, vals ...uint32) tag { // num, den pairs
	var b []byte
	for _, v := range vals {
		b = binary.BigEndian.AppendUint32(b, v)
	}
	return tag{id, 5, uint32(len(vals) / 2), b}
}

// buildTIFF lays out IFD0 and a GPS IFD (big endian). The GPS pointer is patched in.
func buildTIFF(ifd0 []tag, gps []tag) []byte {
	var out []byte
	out = append(out, "MM\x00*"...)
	out = binary.BigEndian.AppendUint32(out, 8)

	writeIFD := func(tags []tag, next uint32) (start int, patch map[uint16]int) {
		start = len(out)
		patch = map[uint16]int{}
		out = binary.BigEndian.AppendUint16(out, uint16(len(tags)))
		dataOff := len(out) + len(tags)*12 + 4
		var data []byte
		for _, t := range tags {
			out = binary.BigEndian.AppendUint16(out, t.id)
			out = binary.BigEndian.AppendUint16(out, t.typ)
			out = binary.BigEndian.AppendUint32(out, t.count)
			patch[t.id] = len(out)
			if len(t.data) <= 4 {
				v := make([]byte, 4)
				copy(v, t.data)
				out = append(out, v...)
			} else {
				out = binary.BigEndian.AppendUint32(out, uint32(dataOff+len(data)))
				data = append(data, t.data...)
			}
		}
		out = binary.BigEndian.AppendUint32(out, next)
		out = append(out, data...)
		return start, patch
	}

	ifd0 = append(ifd0, long(0x8825, 0))
	_, patch := writeIFD(ifd0, 0)
	gpsStart, _ := writeIFD(gps, 0)
	binary.BigEndian.PutUint32(out[patch[0x8825]:], uint32(gpsStart))
	return out
}

func sampleTIFF() []byte {
	return buildTIFF(
		[]tag{ascii(0x010F, "Apple"), ascii(0x0110, "iPhone 13"), short(0x0112, 6), ascii(0x0132, "2024:05:01 10:22:01")},
		[]tag{
			ascii(1, "S"), rationals(2, 23, 1, 33, 1, 0, 1),
			ascii(3, "W"), rationals(4, 46, 1, 38, 1, 0, 1),
		},
	)
}

func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := range 24 {
		for x := range 32 {
			img.Set(x, y, color.RGBA{uint8(x * 8), uint8(y * 10), 128, 255})
		}
	}
	return img
}

func segment(marker byte, payload []byte) []byte {
	b := []byte{0xFF, marker}
	b = binary.BigEndian.AppendUint16(b, uint16(len(payload)+2))
	return append(b, payload...)
}

func dirtyJPEG(t *testing.T) (dirty, clean []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	clean = buf.Bytes()

	var d []byte
	d = append(d, clean[:2]...) // SOI
	d = append(d, segment(0xE1, append(append([]byte{}, exifPrefix...), sampleTIFF()...))...)
	d = append(d, segment(0xE1, append(append([]byte{}, xmpPrefix...), "<x:xmpmeta>author</x:xmpmeta>"...))...)
	d = append(d, segment(0xE2, append(append([]byte{}, iccPrefix...), 1, 1, 0, 0))...)
	d = append(d, segment(0xFE, []byte("shot by Fulano"))...)
	d = append(d, segment(0xED, []byte("Photoshop 3.0\x00IPTC"))...)
	d = append(d, clean[2:]...)
	d = append(d, "TRAILING-PREVIEW-WITH-EXIF"...)
	return d, clean
}

func samePixels(t *testing.T, a, b []byte) {
	t.Helper()
	ia, _, err := image.Decode(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	ib, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("cleaned image does not decode: %v", err)
	}
	if ia.Bounds() != ib.Bounds() {
		t.Fatal("bounds differ")
	}
	for y := ia.Bounds().Min.Y; y < ia.Bounds().Max.Y; y++ {
		for x := ia.Bounds().Min.X; x < ia.Bounds().Max.X; x++ {
			if ia.At(x, y) != ib.At(x, y) {
				t.Fatalf("pixel (%d,%d) differs", x, y)
			}
		}
	}
}

func kinds(items []Item) string {
	var k []string
	for _, it := range items {
		k = append(k, it.Kind)
	}
	return strings.Join(k, ",")
}

// --- tests ------------------------------------------------------------------

func TestJPEG(t *testing.T) {
	dirty, clean := dirtyJPEG(t)

	res, err := Strip(dirty, Options{KeepOrientation: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(res.Removed); got != "EXIF,XMP,Comment,IPTC/Photoshop,Trailing data" {
		t.Fatalf("removed: %s", got)
	}
	exif := res.Removed[0].Detail
	for _, want := range []string{"GPS -23.550000, -46.633333", "camera Apple iPhone 13", "date 2024:05:01"} {
		if !strings.Contains(exif, want) {
			t.Errorf("EXIF summary %q lacks %q", exif, want)
		}
	}
	if got := kinds(res.Kept); got != "ICC profile,EXIF" {
		t.Fatalf("kept: %s", got)
	}

	for _, leak := range []string{"Apple", "Fulano", "author", "TRAILING", "IPTC"} {
		if bytes.Contains(res.Data, []byte(leak)) {
			t.Errorf("cleaned file still contains %q", leak)
		}
	}
	samePixels(t, clean, res.Data)

	// The orientation survives, and only it.
	again, err := Strip(res.Data, Options{KeepOrientation: true})
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed() || kinds(again.Kept) != "ICC profile,EXIF" {
		t.Fatalf("an already stripped file must count as clean: %+v", again)
	}
	if all, _ := Strip(res.Data, Options{}); kinds(all.Removed) != "EXIF" || all.Removed[0].Detail != "orientation only" {
		t.Fatalf("--all must still remove the orientation: %+v", all.Removed)
	}
	if !bytes.Equal(again.Data, res.Data) {
		t.Fatal("stripping must be idempotent")
	}

	// Without KeepOrientation the image is completely bare.
	bare, _ := Strip(dirty, Options{})
	if bytes.Contains(bare.Data, exifPrefix) {
		t.Fatal("EXIF left with KeepOrientation=false")
	}
	samePixels(t, clean, bare.Data)

	if _, err := Strip(dirty[:len(dirty)/2], Options{}); err == nil {
		t.Fatal("truncated JPEG should fail")
	}
}

func pngChunk(typ string, data []byte) []byte {
	c := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	c = append(c, typ...)
	c = append(c, data...)
	return binary.BigEndian.AppendUint32(c, crc32.ChecksumIEEE(c[4:]))
}

func TestPNG(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, testImage())
	clean := buf.Bytes()

	// Insert metadata chunks right after IHDR (8 sig + 25 IHDR).
	var dirty []byte
	dirty = append(dirty, clean[:33]...)
	dirty = append(dirty, pngChunk("tEXt", []byte("Author\x00Fulano de Tal"))...)
	dirty = append(dirty, pngChunk("eXIf", sampleTIFF())...)
	dirty = append(dirty, pngChunk("tIME", []byte{7, 232, 5, 1, 10, 22, 1})...)
	dirty = append(dirty, pngChunk("iDOT", make([]byte, 28))...)
	dirty = append(dirty, clean[33:]...)

	res, err := Strip(dirty, Options{KeepOrientation: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(res.Removed); got != "Text,EXIF,Timestamp,Chunk iDOT" {
		t.Fatalf("removed: %s", got)
	}
	if bytes.Contains(res.Data, []byte("Fulano")) || bytes.Contains(res.Data, []byte("Apple")) {
		t.Fatal("metadata left in PNG")
	}
	samePixels(t, clean, res.Data) // png.Decode also verifies every chunk CRC
}

func TestWebP(t *testing.T) {
	chunk := func(id string, data []byte) []byte {
		c := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
		c = append(c, data...)
		if len(data)%2 == 1 {
			c = append(c, 0)
		}
		return c
	}
	vp8x := make([]byte, 10)
	vp8x[0] = vp8xFlagEXIF | vp8xFlagXMP

	body := []byte("WEBP")
	body = append(body, chunk("VP8X", vp8x)...)
	body = append(body, chunk("VP8L", []byte("fake-image-data"))...)
	body = append(body, chunk("EXIF", sampleTIFF())...)
	body = append(body, chunk("XMP ", []byte("<x:xmpmeta>Fulano</x:xmpmeta>"))...)
	file := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
	file = append(file, body...)

	res, err := Strip(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(res.Removed); got != "EXIF,XMP" {
		t.Fatalf("removed: %s", got)
	}
	if bytes.Contains(res.Data, []byte("Fulano")) || bytes.Contains(res.Data, []byte("Apple")) {
		t.Fatal("metadata left in WebP")
	}
	if size := binary.LittleEndian.Uint32(res.Data[4:]); int(size) != len(res.Data)-8 {
		t.Fatalf("RIFF size %d, file %d", size, len(res.Data))
	}
	if flags := res.Data[20]; flags&(vp8xFlagEXIF|vp8xFlagXMP) != 0 {
		t.Fatalf("VP8X flags not cleared: %08b", flags)
	}
	if !bytes.Contains(res.Data, []byte("fake-image-data")) {
		t.Fatal("image data lost")
	}

	kept, _ := Strip(file, Options{KeepOrientation: true})
	if kept.Data[20]&vp8xFlagEXIF == 0 || kinds(kept.Kept) != "EXIF" {
		t.Fatal("orientation EXIF not kept with its flag")
	}
}

func TestOOXML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)})
		io.WriteString(w, body)
	}
	add("[Content_Types].xml", `<Types/>`)
	add("word/document.xml", `<w:document>Hello</w:document>`)
	add("word/comments.xml", `<w:comments/>`)
	add("docProps/core.xml", `<cp:coreProperties xmlns:cp="c" xmlns:dc="d"><dc:creator>Fulano de Tal</dc:creator><cp:lastModifiedBy>Beltrano</cp:lastModifiedBy><dc:title></dc:title></cp:coreProperties>`)
	add("docProps/app.xml", `<Properties><Company>ACME Ltda</Company><Pages>3</Pages><TotalTime>42</TotalTime></Properties>`)
	photo, _ := dirtyJPEG(t)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "word/media/image1.jpeg", Method: zip.Store})
	w.Write(photo)
	add("docProps/custom.xml", `<Properties><property name="ClientName"><vt:lpwstr>Secret Corp</vt:lpwstr></property></Properties>`)
	zw.SetComment("made by Fulano")
	zw.Close()

	res, err := Strip(buf.Bytes(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "DOCX" {
		t.Fatalf("format %s", res.Format)
	}
	details := ""
	for _, it := range res.Removed {
		details += it.Kind + "=" + it.Detail + ";"
	}
	for _, want := range []string{"Embedded images=image1.jpeg: EXIF GPS -23.550000", "creator: Fulano de Tal", "lastModifiedBy: Beltrano", "Company: ACME Ltda", "TotalTime: 42", "Custom properties=ClientName", "Zip comment", "Timestamps"} {
		if !strings.Contains(details, want) {
			t.Errorf("missing %q in %s", want, details)
		}
	}
	if strings.Contains(details, "Pages") {
		t.Error("statistics should not be reported")
	}
	if kinds(res.Kept) != "Not covered" {
		t.Error("comments.xml should be flagged as not covered")
	}

	zr, err := zip.NewReader(bytes.NewReader(res.Data), int64(len(res.Data)))
	if err != nil {
		t.Fatal(err)
	}
	if zr.Comment != "" {
		t.Fatal("zip comment kept")
	}
	for _, f := range zr.File {
		if !f.Modified.Equal(zipEpoch) {
			t.Errorf("%s: timestamp %v", f.Name, f.Modified)
		}
		rc, _ := f.Open()
		body, err := io.ReadAll(rc) // also verifies CRC
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if f.Name == "word/media/image1.jpeg" {
			if f.Method != zip.Store {
				t.Error("image compression method changed")
			}
			if _, err := jpeg.Decode(bytes.NewReader(body)); err != nil {
				t.Errorf("embedded image broken: %v", err)
			}
		}
		for _, leak := range []string{"Fulano", "Beltrano", "ACME", "Secret Corp", "Apple"} {
			if strings.Contains(string(body), leak) {
				t.Errorf("%s still contains %q", f.Name, leak)
			}
		}
		if f.Name == "word/document.xml" && string(body) != `<w:document>Hello</w:document>` {
			t.Error("document content changed")
		}
	}

	// A plain zip is not an Office document.
	buf.Reset()
	zw = zip.NewWriter(&buf)
	add("notes.txt", "hi")
	zw.Close()
	if _, err := Strip(buf.Bytes(), Options{}); err != ErrUnsupported {
		t.Fatalf("plain zip: %v", err)
	}
}

func TestUnsupportedAndGarbage(t *testing.T) {
	if Detect([]byte("hello world, plain text")) != "" {
		t.Fatal("text detected as supported")
	}
	if _, err := Strip([]byte("hello"), Options{}); err != ErrUnsupported {
		t.Fatal(err)
	}
	// Malformed inputs must error, never panic.
	for _, b := range [][]byte{
		{0xFF, 0xD8, 0xFF},
		{0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF},
		append(append([]byte{}, pngSig...), 0, 0, 0, 99),
		[]byte("RIFF\xff\xff\xff\xffWEBPVP8X"),
	} {
		Strip(b, Options{KeepOrientation: true})
	}
	parseExif([]byte("Exif\x00\x00MM\x00*\xff\xff\xff\xff"))
}

func FuzzStrip(f *testing.F) {
	dirty, _ := dirtyJPEG(&testing.T{})
	f.Add(dirty)
	f.Add(append(append([]byte{}, pngSig...), pngChunk("eXIf", sampleTIFF())...))
	mp4, _, _ := buildMP4(&testing.T{})
	f.Add(mp4)
	heic, _ := buildHEIC(&testing.T{})
	f.Add(heic)
	f.Add(richPDF())
	f.Add(objStmPDF())
	f.Fuzz(func(t *testing.T, b []byte) {
		res, err := Strip(b, Options{KeepOrientation: true})
		if err == nil && res.Data == nil {
			t.Fatal("nil data without error")
		}
	})
}
