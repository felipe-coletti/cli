package metadata

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"fmt"
	"io"
	"strings"
	"testing"
)

// pdfBuilder writes objects and a classic xref table.
type pdfBuilder struct {
	buf     bytes.Buffer
	offsets map[int]int
}

func newPDF() *pdfBuilder {
	b := &pdfBuilder{offsets: map[int]int{}}
	b.buf.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	return b
}

func (b *pdfBuilder) obj(n int, body string) {
	b.offsets[n] = b.buf.Len()
	fmt.Fprintf(&b.buf, "%d 0 obj\n%s\nendobj\n", n, body)
}

func (b *pdfBuilder) stream(n int, dict string, data []byte) {
	b.offsets[n] = b.buf.Len()
	fmt.Fprintf(&b.buf, "%d 0 obj\n<< %s /Length %d >>\nstream\n", n, dict, len(data))
	b.buf.Write(data)
	b.buf.WriteString("\nendstream\nendobj\n")
}

func (b *pdfBuilder) trailer(size int, extra string, prev int) {
	xref := b.buf.Len()
	b.buf.WriteString("xref\n")
	for n := range size {
		if off, ok := b.offsets[n]; ok {
			fmt.Fprintf(&b.buf, "%d 1\n%010d 00000 n \n", n, off)
		}
	}
	p := ""
	if prev > 0 {
		p = fmt.Sprintf(" /Prev %d", prev)
	}
	fmt.Fprintf(&b.buf, "trailer\n<< /Size %d /Root 1 0 R %s%s >>\nstartxref\n%d\n%%%%EOF\n", size, extra, p, xref)
	b.offsets = map[int]int{}
}

func deflate(b []byte) []byte {
	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	zw.Write(b)
	zw.Close()
	return out.Bytes()
}

const sampleXMP = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF><rdf:Description><dc:creator><rdf:Seq><rdf:li>Fulano de Tal</rdf:li></rdf:Seq></dc:creator><xmp:CreatorTool>Word</xmp:CreatorTool></rdf:Description></rdf:RDF></x:xmpmeta>`

// richPDF has an Info dict with a value in a separate object, a compressed
// XMP stream, an ID, a comment annotation, a form field and an incremental
// update that changed the author (the old revision still holds the old one).
func richPDF() []byte {
	b := newPDF()
	b.obj(1, "<< /Type /Catalog /Pages 2 0 R /Metadata 7 0 R /AcroForm << /Fields [6 0 R] >> >>")
	b.obj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	b.obj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Annots [5 0 R 6 0 R] >>")
	b.obj(4, "<< /Author (Fulano de Tal) /Title 8 0 R /Producer (Word \\(16\\)) /CreationDate (D:20240501102201Z) >>")
	b.obj(5, "<< /Type /Annot /Subtype /Text /Rect [0 0 1 1] /T (Ciclano) /Contents (ok) >>")
	b.obj(6, "<< /Type /Annot /Subtype /Widget /FT /Tx /T (campo_nome) /Rect [0 0 1 1] >>")
	b.stream(7, "/Type /Metadata /Subtype /XML /Filter /FlateDecode", deflate([]byte(sampleXMP)))
	b.obj(8, "(Relatorio Secreto)")
	b.trailer(9, "/Info 4 0 R /ID [<00112233445566778899AABBCCDDEEFF> <00112233445566778899AABBCCDDEEFF>]", 0)
	first := b.buf.Len()

	// Incremental update: a new Info object replaces the old one.
	b.obj(4, "<< /Author (Beltrano) >>")
	b.trailer(9, "/Info 4 0 R /ID [<00112233445566778899AABBCCDDEEFF> <FFEEDDCCBBAA99887766554433221100>]", first)
	return b.buf.Bytes()
}

func TestPDF(t *testing.T) {
	file := richPDF()
	res, err := Strip(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string]string{}
	for _, it := range res.Removed {
		byKind[it.Kind] = it.Detail
	}
	if len(byKind) != 4 || byKind["Document ID"] == "" || byKind["Annotation authors"] != "Ciclano" {
		t.Fatalf("removed: %+v", res.Removed)
	}
	info := byKind["Document info"]
	for _, want := range []string{"Author: Fulano de Tal", "Title: Relatorio Secreto", "Producer: Word (16)", "CreationDate: 2024-05-01 10:22:01", "Author: Beltrano"} {
		if !strings.Contains(info, want) {
			t.Errorf("info summary %q lacks %q", info, want)
		}
	}
	if xmp := byKind["XMP"]; !strings.Contains(xmp, "creator: Fulano de Tal") || !strings.Contains(xmp, "CreatorTool: Word") {
		t.Errorf("XMP summary: %q", xmp)
	}

	if len(res.Data) != len(file) {
		t.Fatal("size changed: xref offsets would break")
	}
	for _, leak := range []string{"Fulano", "Beltrano", "Ciclano", "Secreto", "Word", "AABBCC"} {
		if bytes.Contains(res.Data, []byte(leak)) {
			t.Errorf("cleaned PDF still contains %q", leak)
		}
	}
	if !bytes.Contains(res.Data, []byte("(campo_nome)")) {
		t.Error("form field names must be kept")
	}
	if !bytes.Contains(res.Data, []byte("(ok)")) {
		t.Error("annotation contents must be kept")
	}

	// Every object must still start exactly where the xref says.
	d := &pdfDoc{file: res.Data, out: res.Data, byNum: map[int][]*pdfObj{}}
	d.scan()
	if len(d.objs) != 9 {
		t.Fatalf("cleaned PDF has %d objects, want 9", len(d.objs))
	}

	again, err := Strip(res.Data, Options{})
	if err != nil || again.Changed() {
		t.Fatalf("second pass: %v %+v", err, again.Removed)
	}
}

func objStmPDF() []byte {
	inner := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [] /Count 0 >>",
		3: "<< /Author <FEFF00460075006C0061006E006F> /Producer (Gerador) >>",
		4: "<< /Type /Annot /Subtype /Highlight /T (Beltrano) >>",
	}
	var header, body string
	for n := 1; n <= 4; n++ {
		header += fmt.Sprintf("%d %d ", n, len(body))
		body += inner[n] + "\n"
	}
	b := newPDF()
	b.stream(5, fmt.Sprintf("/Type /ObjStm /N 4 /First %d /Filter /FlateDecode", len(header)), deflate([]byte(header+body)))
	b.trailer(6, "/Info 3 0 R", 0)
	return b.buf.Bytes()
}

func TestPDFObjectStream(t *testing.T) {
	file := objStmPDF()
	res, err := Strip(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(res.Removed); got != "Document info,Annotation authors" {
		t.Fatalf("removed %s, kept %+v", got, res.Kept)
	}
	if !strings.Contains(res.Removed[0].Detail, "Author: Fulano") {
		t.Fatalf("UTF-16 author not decoded: %q", res.Removed[0].Detail)
	}
	if len(res.Data) != len(file) {
		t.Fatal("size changed")
	}

	// Decompress the rewritten object stream and check its content.
	d := &pdfDoc{file: res.Data, out: res.Data, byNum: map[int][]*pdfObj{}}
	d.scan()
	stm := d.byNum[5][0]
	zr, err := zlib.NewReader(bytes.NewReader(res.Data[stm.dataStart:stm.dataEnd]))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("recompressed stream is invalid: %v", err)
	}
	if bytes.Contains(plain, []byte("Beltrano")) || bytes.Contains(plain, []byte("0046")) || !bytes.Contains(plain, []byte("/Catalog")) {
		t.Fatalf("bad object stream content: %q", plain)
	}

	again, _ := Strip(res.Data, Options{})
	if again.Changed() {
		t.Fatalf("second pass: %+v", again.Removed)
	}
}

func TestFlateExact(t *testing.T) {
	data := bytes.Repeat([]byte("1 0 obj << /A (x) >> "), 400)
	for _, size := range []int{60, 100, 5000, 70000, 140000} {
		out, ok := flateExact(data, size)
		if !ok {
			if size >= 100 {
				t.Fatalf("size %d: did not fit", size)
			}
			continue
		}
		if len(out) != size {
			t.Fatalf("size %d: got %d bytes", size, len(out))
		}
		zr, err := zlib.NewReader(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		plain, err := io.ReadAll(zr)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if !bytes.HasPrefix(plain, data) || strings.TrimSpace(string(plain[len(data):])) != "" {
			t.Fatalf("size %d: content altered", size)
		}
	}
}

func TestPDFEncryptedAndSigned(t *testing.T) {
	b := newPDF()
	b.obj(1, "<< /Type /Catalog >>")
	b.obj(2, "<< /Author (cifrado) >>")
	b.obj(3, "<< /Filter /Standard /V 2 >>")
	b.trailer(4, "/Info 2 0 R /Encrypt 3 0 R", 0)
	enc := b.buf.Bytes()
	res, err := Strip(enc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed() || !bytes.Equal(res.Data, enc) || kinds(res.Kept) != "Not covered" {
		t.Fatalf("encrypted PDF must be left untouched and flagged: %+v", res)
	}

	b = newPDF()
	b.obj(1, "<< /Type /Catalog >>")
	b.obj(2, "<< /Author (Fulano) >>")
	b.obj(3, "<< /Type /Sig /ByteRange [0 10 20 30] /Contents <00> >>")
	b.trailer(4, "/Info 2 0 R", 0)
	res, err = Strip(b.buf.Bytes(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if kinds(res.Kept) != "Signature" {
		t.Fatalf("signed PDF must warn: %+v", res.Kept)
	}
}

func TestPDFGarbage(t *testing.T) {
	file := richPDF()
	for _, b := range [][]byte{
		[]byte("%PDF-1.4\n"),
		[]byte("%PDF-1.4\n1 0 obj << /A (unterminated"),
		[]byte("%PDF-1.4\n1 0 obj [[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[["),
		file[:len(file)/2],
	} {
		Strip(b, Options{}) // must not panic
	}
}

func TestPDFEmbeddedJPEG(t *testing.T) {
	dirty, clean := dirtyJPEG(t)

	encodings := map[string]func([]byte) []byte{
		"/DCTDecode": func(b []byte) []byte { return b },
		"[/ASCII85Decode /DCTDecode]": func(b []byte) []byte {
			// Line-wrapped like real encoders, with the "~>" terminator.
			var out bytes.Buffer
			enc := ascii85.NewEncoder(&out)
			enc.Write(b)
			enc.Close()
			s := out.String()
			var wrapped strings.Builder
			for i := 0; i < len(s); i += 64 {
				wrapped.WriteString(s[i:min(len(s), i+64)] + "\n")
			}
			return []byte(wrapped.String() + "~>")
		},
		"[/ASCIIHexDecode /DCTDecode]": func(b []byte) []byte { return []byte(fmt.Sprintf("%x>", b)) },
		"[/FlateDecode /DCTDecode]":    deflate,
	}
	for filter, encode := range encodings {
		t.Run(filter, func(t *testing.T) {
			b := newPDF()
			b.obj(1, "<< /Type /Catalog >>")
			b.stream(2, "/Type /XObject /Subtype /Image /Width 32 /Height 24 /Filter "+filter, encode(dirty))
			b.trailer(3, "", 0)
			file := b.buf.Bytes()

			res, err := Strip(file, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if kinds(res.Removed) != "Embedded images" || !strings.Contains(res.Removed[0].Detail, "GPS -23.550000") {
				t.Fatalf("removed %+v kept %+v", res.Removed, res.Kept)
			}
			if len(res.Data) != len(file) {
				t.Fatal("size changed")
			}

			// Decode the stream back and check the JPEG.
			d := &pdfDoc{file: res.Data, out: res.Data, byNum: map[int][]*pdfObj{}}
			d.scan()
			o := d.byNum[2][0]
			data := res.Data[o.dataStart:o.dataEnd]
			for _, f := range filterNames(o.val.get("Filter")) {
				if f == "DCTDecode" {
					break
				}
				if data, err = pdfDecode(f, data); err != nil {
					t.Fatalf("%s no longer decodes: %v", f, err)
				}
			}
			if bytes.Contains(data, []byte("Apple")) || bytes.Contains(data, []byte("Fulano")) {
				t.Fatal("metadata left in embedded image")
			}
			samePixels(t, clean, data)

			if again, _ := Strip(res.Data, Options{}); again.Changed() {
				t.Fatalf("second pass: %+v", again.Removed)
			}
		})
	}
}

func TestA85(t *testing.T) {
	for _, data := range [][]byte{nil, {1}, {0, 0, 0, 0}, []byte("hello world!"), bytes.Repeat([]byte{0, 0, 0, 0, 9}, 50)} {
		enc := append(encodeA85(data), "~>"...)
		dec, err := decodeA85(enc)
		if err != nil || !bytes.Equal(dec, data) {
			t.Fatalf("%x: round trip %x %v", data, dec, err)
		}
		// Must match the standard library decoder.
		std := make([]byte, len(data)+8)
		n, _, err := ascii85.Decode(std, encodeA85(data), true)
		if err != nil || !bytes.Equal(std[:n], data) {
			t.Fatalf("%x: stdlib disagrees: %x %v", data, std[:n], err)
		}
	}
}
