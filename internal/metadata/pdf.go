package metadata

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/adler32"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

// PDF support.
//
// A PDF's cross-reference table stores the byte offset of every object, so
// nothing may move. Metadata is therefore overwritten with same-length
// content: the document information dictionary becomes "<<   >>", XMP
// streams get an empty XMP packet padded with spaces, the document ID is
// zeroed, annotation authors are blanked and embedded JPEG photos lose their
// EXIF/XMP (their metadata segments become zero-filled comments). The whole file is scanned
// object by object, so old revisions kept by incremental updates (which
// still contain the previous author, title, ...) are cleaned too. Objects
// stored inside compressed object streams are edited after decompression
// and recompressed to exactly the original length.

func isPDF(b []byte) bool { return bytes.HasPrefix(b, []byte("%PDF-")) }

// ---------------------------------------------------------------------------
// Lexer / parser

type pdfVal struct {
	kind       byte // 'd' dict, 'a' array, 's' (string), 'h' <hex>, 'n' /name, 'r' ref, 'k' other token
	start, end int
	dict       []pdfKV
	arr        []*pdfVal
	text       string // name (without '/') or token
	ref        int    // object number for 'r'
}

type pdfKV struct {
	key      string
	keyStart int
	val      *pdfVal
}

func (v *pdfVal) get(key string) *pdfVal {
	if v == nil || v.kind != 'd' {
		return nil
	}
	for _, kv := range v.dict {
		if kv.key == key {
			return kv.val
		}
	}
	return nil
}

func (v *pdfVal) name(key string) string {
	if n := v.get(key); n != nil && n.kind == 'n' {
		return n.text
	}
	return ""
}

type pdfLex struct {
	b     []byte
	i     int
	depth int
}

func isPDFSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0
}

func isPDFDelim(c byte) bool { return strings.IndexByte("()<>[]{}/%", c) >= 0 }

func (p *pdfLex) ws() {
	for p.i < len(p.b) {
		c := p.b[p.i]
		switch {
		case isPDFSpace(c):
			p.i++
		case c == '%':
			for p.i < len(p.b) && p.b[p.i] != '\n' && p.b[p.i] != '\r' {
				p.i++
			}
		default:
			return
		}
	}
}

func (p *pdfLex) token() string {
	start := p.i
	for p.i < len(p.b) && !isPDFSpace(p.b[p.i]) && !isPDFDelim(p.b[p.i]) {
		p.i++
	}
	return string(p.b[start:p.i])
}

func isInt(s string) bool {
	if s == "" || len(s) > 10 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

var errBadPDF = fmt.Errorf("invalid PDF syntax")

func (p *pdfLex) value() (*pdfVal, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 64 {
		return nil, errBadPDF
	}

	p.ws()
	if p.i >= len(p.b) {
		return nil, errBadPDF
	}
	v := &pdfVal{start: p.i}

	switch c := p.b[p.i]; {
	case c == '<' && p.i+1 < len(p.b) && p.b[p.i+1] == '<':
		v.kind = 'd'
		p.i += 2
		for {
			p.ws()
			if p.i+1 < len(p.b) && p.b[p.i] == '>' && p.b[p.i+1] == '>' {
				p.i += 2
				break
			}
			if p.i >= len(p.b) || p.b[p.i] != '/' {
				return nil, errBadPDF
			}
			keyStart := p.i
			p.i++
			key := p.token()
			val, err := p.value()
			if err != nil {
				return nil, err
			}
			v.dict = append(v.dict, pdfKV{key, keyStart, val})
			if len(v.dict) > 10000 {
				return nil, errBadPDF
			}
		}

	case c == '<':
		v.kind = 'h'
		end := bytes.IndexByte(p.b[p.i:], '>')
		if end < 0 {
			return nil, errBadPDF
		}
		p.i += end + 1

	case c == '[':
		v.kind = 'a'
		p.i++
		for {
			p.ws()
			if p.i < len(p.b) && p.b[p.i] == ']' {
				p.i++
				break
			}
			el, err := p.value()
			if err != nil {
				return nil, err
			}
			v.arr = append(v.arr, el)
			if len(v.arr) > 1<<20 {
				return nil, errBadPDF
			}
		}

	case c == '(':
		v.kind = 's'
		level := 0
		for ; p.i < len(p.b); p.i++ {
			switch p.b[p.i] {
			case '\\':
				p.i++
			case '(':
				level++
			case ')':
				level--
			}
			if level == 0 {
				break
			}
		}
		if p.i >= len(p.b) {
			return nil, errBadPDF
		}
		p.i++

	case c == '/':
		v.kind = 'n'
		p.i++
		v.text = p.token()

	case c == ')' || c == '>' || c == ']' || c == '{' || c == '}':
		return nil, errBadPDF

	default:
		v.kind = 'k'
		v.text = p.token()
		if v.text == "" {
			return nil, errBadPDF
		}
		// "N G R" is an indirect reference.
		if isInt(v.text) {
			save := p.i
			p.ws()
			gen := p.token()
			p.ws()
			if isInt(gen) && p.i < len(p.b) && p.b[p.i] == 'R' && (p.i+1 == len(p.b) || isPDFSpace(p.b[p.i+1]) || isPDFDelim(p.b[p.i+1])) {
				p.i++
				v.kind = 'r'
				v.ref, _ = strconv.Atoi(v.text)
			} else {
				p.i = save
			}
		}
	}

	v.end = p.i
	return v, nil
}

// ---------------------------------------------------------------------------
// Document scan

type pdfObj struct {
	num        int
	buf        int // 0 = file, n>0 = decompressed object stream n-1
	val        *pdfVal
	dataStart  int // stream data, file offsets (-1 if not a stream)
	dataEnd    int
	lengthRef  int
	directSize bool
}

type pdfStm struct {
	obj   *pdfObj
	data  []byte // decompressed, edited in place
	dirty bool
}

type pdfDoc struct {
	file     []byte
	out      []byte // file copy, edited in place
	objs     []*pdfObj
	byNum    map[int][]*pdfObj
	trailers []*pdfVal // trailer dictionaries and xref stream dictionaries
	stms     []*pdfStm
}

func (d *pdfDoc) buf(n int) []byte {
	if n == 0 {
		return d.out
	}
	return d.stms[n-1].data
}

func (d *pdfDoc) scan() {
	p := &pdfLex{b: d.file}
	for p.i < len(d.file) {
		c := d.file[p.i]
		switch {
		case c >= '0' && c <= '9' && (p.i == 0 || !isRegular(d.file[p.i-1])):
			if !d.object(p) {
				p.i++
			}
		case c == 't' && bytes.HasPrefix(d.file[p.i:], []byte("trailer")):
			p.i += 7
			if v, err := p.value(); err == nil && v.kind == 'd' {
				d.trailers = append(d.trailers, v)
			}
		default:
			p.i++
		}
	}
}

func isRegular(c byte) bool { return !isPDFSpace(c) && !isPDFDelim(c) }

// object parses "N G obj ... endobj" at p.i, returning false if there is none.
func (d *pdfDoc) object(p *pdfLex) bool {
	save := p.i
	num := p.token()
	p.ws()
	gen := p.token()
	p.ws()
	if !isInt(num) || !isInt(gen) || !bytes.HasPrefix(d.file[p.i:], []byte("obj")) {
		p.i = save
		return false
	}
	p.i += 3

	val, err := p.value()
	if err != nil {
		p.i = save
		return false
	}

	o := &pdfObj{buf: 0, val: val, dataStart: -1}
	o.num, _ = strconv.Atoi(num)

	p.ws()
	if bytes.HasPrefix(d.file[p.i:], []byte("stream")) && val.kind == 'd' {
		p.i += 6
		if p.i < len(d.file) && d.file[p.i] == '\r' {
			p.i++
		}
		if p.i < len(d.file) && d.file[p.i] == '\n' {
			p.i++
		}
		o.dataStart = p.i

		length := val.get("Length")
		switch {
		case length != nil && length.kind == 'k' && isInt(length.text):
			n, _ := strconv.Atoi(length.text)
			if end := o.dataStart + n; end <= len(d.file) && endstreamFollows(d.file, end) {
				o.dataEnd, o.directSize = end, true
			}
		case length != nil && length.kind == 'r':
			o.lengthRef = length.ref
		}
		if !o.directSize {
			e := bytes.Index(d.file[o.dataStart:], []byte("endstream"))
			if e < 0 {
				p.i = save
				return false
			}
			o.dataEnd = trimEOL(d.file, o.dataStart, o.dataStart+e)
		}
		p.i = o.dataEnd
	}

	if e := bytes.Index(d.file[p.i:min(len(d.file), p.i+1<<20)], []byte("endobj")); e >= 0 {
		p.i += e + 6
	}

	d.objs = append(d.objs, o)
	d.byNum[o.num] = append(d.byNum[o.num], o)
	if val.name("Type") == "XRef" {
		d.trailers = append(d.trailers, val)
	}
	return true
}

func endstreamFollows(b []byte, i int) bool {
	for i < len(b) && isPDFSpace(b[i]) {
		i++
	}
	return bytes.HasPrefix(b[i:], []byte("endstream"))
}

func trimEOL(b []byte, start, end int) int {
	if end > start && b[end-1] == '\n' {
		end--
	}
	if end > start && b[end-1] == '\r' {
		end--
	}
	return end
}

// resolveLengths fixes stream bounds whose /Length was an indirect reference.
func (d *pdfDoc) resolveLengths() {
	for _, o := range d.objs {
		if o.directSize || o.lengthRef == 0 {
			continue
		}
		for _, lo := range d.byNum[o.lengthRef] {
			if lo.val.kind == 'k' && isInt(lo.val.text) {
				n, _ := strconv.Atoi(lo.val.text)
				if end := o.dataStart + n; end <= len(d.file) && endstreamFollows(d.file, end) {
					o.dataEnd, o.directSize = end, true
				}
			}
		}
	}
}

// expandObjectStreams decompresses /ObjStm streams and registers the
// objects they contain.
func (d *pdfDoc) expandObjectStreams() []Item {
	var notCovered []Item
	for _, o := range d.objs {
		if o.buf != 0 || o.dataStart < 0 || o.val.name("Type") != "ObjStm" {
			continue
		}
		if o.val.name("Filter") != "FlateDecode" || o.val.get("DecodeParms") != nil || !o.directSize {
			notCovered = append(notCovered, Item{"Not covered", "object stream with an unsupported encoding"})
			continue
		}
		zr, err := zlib.NewReader(bytes.NewReader(d.file[o.dataStart:o.dataEnd]))
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(zr, 256<<20))
		if err != nil && len(data) == 0 {
			continue
		}

		stm := &pdfStm{obj: o, data: data}
		d.stms = append(d.stms, stm)
		id := len(d.stms)

		first, _ := strconv.Atoi(tokenText(o.val.get("First")))
		n, _ := strconv.Atoi(tokenText(o.val.get("N")))
		hp := &pdfLex{b: data}
		for range min(n, 1<<20) {
			hp.ws()
			num := hp.token()
			hp.ws()
			off := hp.token()
			if !isInt(num) || !isInt(off) {
				break
			}
			o2, _ := strconv.Atoi(num)
			off2, _ := strconv.Atoi(off)
			vp := &pdfLex{b: data, i: first + off2}
			if vp.i < 0 || vp.i >= len(data) {
				continue
			}
			val, err := vp.value()
			if err != nil {
				continue
			}
			obj := &pdfObj{num: o2, buf: id, val: val, dataStart: -1}
			d.objs = append(d.objs, obj)
			d.byNum[o2] = append(d.byNum[o2], obj)
		}
	}
	return notCovered
}

func tokenText(v *pdfVal) string {
	if v == nil || v.kind != 'k' {
		return ""
	}
	return v.text
}

// ---------------------------------------------------------------------------
// Cleaning

type pdfStrip struct {
	*pdfDoc
	res     *Result
	removed map[string][]string // kind -> details
	order   []string
	bufOf   map[string]map[int]bool // kind -> buffers it touched
}

func (s *pdfStrip) note(kind, detail string, buf int) {
	if _, ok := s.removed[kind]; !ok {
		s.order = append(s.order, kind)
		s.bufOf[kind] = map[int]bool{}
	}
	if detail != "" {
		s.removed[kind] = append(s.removed[kind], detail)
	} else if s.removed[kind] == nil {
		s.removed[kind] = []string{}
	}
	s.bufOf[kind][buf] = true
	if buf > 0 {
		s.stms[buf-1].dirty = true
	}
}

func (s *pdfStrip) fill(buf, start, end int, with []byte) {
	b := s.buf(buf)
	if start < 0 || end > len(b) || start > end {
		return
	}
	copy(b[start:end], with)
}

func spaces(n int) []byte { return bytes.Repeat([]byte(" "), max(n, 0)) }

// blankString replaces a string token with "()" padded to the same length.
func (s *pdfStrip) blankString(buf int, v *pdfVal) {
	s.fill(buf, v.start, v.end, append([]byte("()"), spaces(v.end-v.start-2)...))
}

var infoLabels = map[string]bool{
	"Title": true, "Author": true, "Subject": true, "Keywords": true, "Creator": true,
	"Producer": true, "CreationDate": true, "ModDate": true, "Company": true, "Trapped": false,
}

func stripPDF(b []byte, _ Options) (*Result, error) {
	d := &pdfDoc{file: b, out: bytes.Clone(b), byNum: map[int][]*pdfObj{}}
	d.scan()
	d.resolveLengths()

	s := &pdfStrip{pdfDoc: d, res: &Result{Format: "PDF"}, removed: map[string][]string{}, bufOf: map[string]map[int]bool{}}

	for _, t := range d.trailers {
		if t.get("Encrypt") != nil {
			s.res.Kept = append(s.res.Kept, Item{"Not covered", "encrypted PDF: metadata cannot be edited without the password"})
			s.res.Data = b
			return s.res, nil
		}
	}
	if len(d.objs) == 0 {
		return nil, errBadPDF
	}

	s.res.Kept = append(s.res.Kept, d.expandObjectStreams()...)

	// Document information dictionaries, from every revision.
	seenInfo := map[int]bool{}
	for _, t := range d.trailers {
		info := t.get("Info")
		switch {
		case info == nil:
		case info.kind == 'd':
			s.blankInfo(0, info)
		case info.kind == 'r' && !seenInfo[info.ref]:
			seenInfo[info.ref] = true
			for _, o := range d.byNum[info.ref] {
				if o.val.kind == 'd' {
					s.blankInfo(o.buf, o.val)
				}
			}
		}
		if id := t.get("ID"); id != nil && id.kind == 'a' {
			s.zeroID(id)
		}
	}

	signed := false
	for _, o := range d.objs {
		v := o.val
		switch {
		case v.kind != 'd':
			continue
		case v.name("Type") == "Metadata" && o.dataStart >= 0:
			s.blankXMP(o)
		case v.name("Subtype") == "Image" && o.dataStart >= 0 && isDCT(v.get("Filter")) && o.directSize:
			s.cleanImage(o)
		case v.name("Type") == "Sig" || v.get("ByteRange") != nil:
			signed = true
		}
		s.annotations(o.buf, v, 0)
	}

	s.finish()
	if signed {
		s.res.Kept = append(s.res.Kept, Item{"Signature", "digitally signed: any change invalidates the signature"})
	}
	return s.res, nil
}

func (s *pdfStrip) blankInfo(buf int, info *pdfVal) {
	if len(info.dict) == 0 {
		return
	}
	for _, kv := range info.dict {
		val := s.display(buf, kv.val)
		if kv.val.kind == 'r' {
			// Values stored as separate objects (e.g. "/Author 12 0 R").
			for _, o := range s.byNum[kv.val.ref] {
				if o.val.kind == 's' || o.val.kind == 'h' {
					if val == "" {
						val = decodePDFString(s.buf(o.buf)[o.val.start:o.val.end])
					}
					s.blankString(o.buf, o.val)
					s.note("Document info", "", o.buf)
				}
			}
		}
		if val == "" {
			continue
		}
		s.note("Document info", kv.key+": "+truncate(val, 50), buf)
	}
	s.fill(buf, info.start+2, info.end-2, spaces(info.end-info.start-4))
	s.note("Document info", "", buf)
}

func (s *pdfStrip) display(buf int, v *pdfVal) string {
	b := s.file
	if buf > 0 {
		b = s.buf(buf)
	}
	switch v.kind {
	case 's', 'h':
		return pdfDate(decodePDFString(b[v.start:v.end]))
	case 'n':
		return v.text
	case 'k':
		return v.text
	}
	return ""
}

func (s *pdfStrip) zeroID(id *pdfVal) {
	changed := false
	for _, el := range id.arr {
		if (el.kind != 'h' && el.kind != 's') || el.end-el.start < 2 {
			continue
		}
		inner := s.out[el.start+1 : el.end-1]
		zeros := bytes.Repeat([]byte("0"), len(inner))
		if el.kind == 'h' {
			// Keep whitespace inside hex strings; zero the digits.
			for i, c := range inner {
				if isPDFSpace(c) {
					zeros[i] = c
				}
			}
		}
		if !bytes.Equal(inner, zeros) {
			copy(inner, zeros)
			changed = true
		}
	}
	if changed {
		s.note("Document ID", "fingerprint shared by every copy and version of the file", 0)
	}
}

var xmpFields = regexp.MustCompile(`(?s)<(dc:creator|xmp:CreatorTool|pdf:Producer|pdf:Author|xmp:CreateDate|xmp:ModifyDate|dc:title|pdf:Keywords)>(.*?)</`)
var xmpTags = regexp.MustCompile(`<[^>]*>`)

func (s *pdfStrip) blankXMP(o *pdfObj) {
	raw := s.file[o.dataStart:o.dataEnd]
	n := len(raw)
	if bytes.Equal(raw, emptyXMP(n)) {
		return
	}

	content := raw
	if o.val.name("Filter") == "FlateDecode" {
		if zr, err := zlib.NewReader(bytes.NewReader(raw)); err == nil {
			content, _ = io.ReadAll(io.LimitReader(zr, 16<<20))
		}
	}
	var details []string
	for _, m := range xmpFields.FindAllSubmatch(content, 8) {
		v := strings.TrimSpace(xmpTags.ReplaceAllString(string(m[2]), " "))
		if v != "" {
			details = append(details, strings.TrimPrefix(strings.SplitN(string(m[1]), ":", 2)[1], "")+": "+truncate(v, 40))
		}
	}
	if len(details) == 0 {
		details = append(details, humanSize(n))
	}

	// The new content is stored unencoded: drop the filter entries.
	for _, kv := range o.val.dict {
		if kv.key == "Filter" || kv.key == "DecodeParms" {
			s.fill(0, kv.keyStart, kv.val.end, spaces(kv.val.end-kv.keyStart))
		}
	}
	s.fill(0, o.dataStart, o.dataEnd, emptyXMP(n))
	for _, d := range details {
		s.note("XMP", d, 0)
	}
}

// filterNames lists a stream's filters in decoding order, expanding the
// abbreviations used by inline images.
func filterNames(f *pdfVal) []string {
	abbrev := map[string]string{"A85": "ASCII85Decode", "AHx": "ASCIIHexDecode", "Fl": "FlateDecode", "DCT": "DCTDecode"}
	var names []string
	add := func(v *pdfVal) {
		if v.kind == 'n' {
			n := v.text
			if a, ok := abbrev[n]; ok {
				n = a
			}
			names = append(names, n)
		}
	}
	switch {
	case f == nil:
	case f.kind == 'n':
		add(f)
	case f.kind == 'a':
		for _, el := range f.arr {
			add(el)
		}
	}
	return names
}

func isDCT(f *pdfVal) bool {
	n := filterNames(f)
	return len(n) > 0 && n[len(n)-1] == "DCTDecode"
}

// cleanImage removes EXIF, XMP, comments, ... from an embedded JPEG. Outer
// ASCII85/ASCIIHex/Flate layers are decoded, the JPEG is wiped in place and
// the layers are re-encoded to exactly their original lengths.
func (s *pdfStrip) cleanImage(o *pdfObj) {
	filters := filterNames(o.val.get("Filter"))
	outer := filters[:len(filters)-1]
	if len(outer) > 0 && o.val.get("DecodeParms") != nil {
		return // predictors etc.: not handled
	}

	layers := [][]byte{s.file[o.dataStart:o.dataEnd]}
	for _, f := range outer {
		dec, err := pdfDecode(f, layers[len(layers)-1])
		if err != nil {
			return
		}
		layers = append(layers, dec)
	}

	img := bytes.Clone(layers[len(layers)-1])
	if !isJPEG(img) {
		return
	}
	items, err := wipeJPEG(img)
	if err != nil || len(items) == 0 {
		return
	}

	enc := img
	for k := len(outer) - 1; k >= 0; k-- {
		var ok bool
		if enc, ok = pdfEncodeExact(outer[k], enc, len(layers[k])); !ok {
			s.res.Kept = append(s.res.Kept, Item{"Not covered", fmt.Sprintf("image %d: could not re-encode without changing its size", o.num)})
			return
		}
	}
	copy(s.out[o.dataStart:o.dataEnd], enc)

	var parts []string
	for _, it := range items {
		if it.Detail != "" && it.Kind == "EXIF" {
			parts = append(parts, "EXIF "+it.Detail)
		} else {
			parts = append(parts, it.Kind)
		}
	}
	s.note("Embedded images", fmt.Sprintf("image %d: %s", o.num, strings.Join(parts, ", ")), 0)
}

func pdfDecode(filter string, b []byte) ([]byte, error) {
	switch filter {
	case "FlateDecode":
		zr, err := zlib.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		out, err := io.ReadAll(io.LimitReader(zr, 256<<20))
		if len(out) == 0 && err != nil {
			return nil, err
		}
		return out, nil
	case "ASCII85Decode":
		return decodeA85(b)
	case "ASCIIHexDecode":
		return decodeAHx(b)
	}
	return nil, fmt.Errorf("unsupported filter %s", filter)
}

// pdfEncodeExact encodes data with filter into exactly size bytes, padding
// with content every decoder ignores (whitespace, or trailing spaces after
// the JPEG/ASCII data inside a deflate stream).
func pdfEncodeExact(filter string, data []byte, size int) ([]byte, bool) {
	var enc []byte
	switch filter {
	case "FlateDecode":
		return flateExact(data, size)
	case "ASCII85Decode":
		enc = encodeA85(data)
	case "ASCIIHexDecode":
		enc = []byte(fmt.Sprintf("%X", data))
	default:
		return nil, false
	}
	eod := []byte("~>")
	if filter == "ASCIIHexDecode" {
		eod = []byte(">")
	}
	if len(enc)+len(eod) > size {
		return nil, false
	}
	out := append(enc, spaces(size-len(enc)-len(eod))...)
	return append(out, eod...), true
}

func decodeAHx(b []byte) ([]byte, error) {
	var out []byte
	var hi int = -1
	for _, c := range b {
		var v int
		switch {
		case c == '>':
			if hi >= 0 {
				out = append(out, byte(hi<<4))
			}
			return out, nil
		case isPDFSpace(c):
			continue
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'f':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v = int(c-'A') + 10
		default:
			return nil, errBadPDF
		}
		if hi < 0 {
			hi = v
		} else {
			out = append(out, byte(hi<<4|v))
			hi = -1
		}
	}
	return out, nil
}

func decodeA85(b []byte) ([]byte, error) {
	var out []byte
	var group [5]byte
	n := 0
	flush := func(count int) {
		var v uint32
		for i := range 5 {
			v = v*85 + uint32(group[i])
		}
		var w [4]byte
		binary.BigEndian.PutUint32(w[:], v)
		out = append(out, w[:count]...)
	}
	b = bytes.TrimPrefix(bytes.TrimLeft(b, " \t\r\n"), []byte("<~"))
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case isPDFSpace(c):
		case c == '~':
			if n > 0 {
				for k := n; k < 5; k++ {
					group[k] = 84
				}
				flush(n - 1)
			}
			return out, nil
		case c == 'z' && n == 0:
			out = append(out, 0, 0, 0, 0)
		case c >= '!' && c <= 'u':
			group[n] = c - '!'
			n++
			if n == 5 {
				flush(4)
				n = 0
			}
		default:
			return nil, errBadPDF
		}
	}
	return nil, errBadPDF // no end-of-data marker
}

// encodeA85 encodes without line breaks, using "z" for zero groups.
func encodeA85(data []byte) []byte {
	var out []byte
	for i := 0; i < len(data); i += 4 {
		var w [4]byte
		n := copy(w[:], data[i:])
		v := binary.BigEndian.Uint32(w[:])
		if n == 4 && v == 0 {
			out = append(out, 'z')
			continue
		}
		var g [5]byte
		for k := 4; k >= 0; k-- {
			g[k] = byte(v%85) + '!'
			v /= 85
		}
		out = append(out, g[:n+1]...)
	}
	return out
}

var markupAnnots = map[string]bool{
	"Text": true, "FreeText": true, "Line": true, "Square": true, "Circle": true, "Polygon": true,
	"PolyLine": true, "Highlight": true, "Underline": true, "Squiggly": true, "StrikeOut": true,
	"Stamp": true, "Caret": true, "Ink": true, "FileAttachment": true, "Sound": true, "Redact": true,
}

// annotations blanks the author (/T) of comment-like annotations. Form
// widgets also use /T (as the field name) and are left alone.
func (s *pdfStrip) annotations(buf int, v *pdfVal, depth int) {
	if v == nil || depth > 8 {
		return
	}
	switch v.kind {
	case 'd':
		if markupAnnots[v.name("Subtype")] {
			if t := v.get("T"); t != nil && (t.kind == 's' || t.kind == 'h') {
				if name := strings.TrimSpace(decodePDFString(s.buf(buf)[t.start:t.end])); name != "" {
					s.blankString(buf, t)
					s.note("Annotation authors", truncate(name, 30), buf)
				}
			}
		}
		for _, kv := range v.dict {
			if kv.val.kind == 'd' || kv.val.kind == 'a' {
				s.annotations(buf, kv.val, depth+1)
			}
		}
	case 'a':
		for _, el := range v.arr {
			if el.kind == 'd' {
				s.annotations(buf, el, depth+1)
			}
		}
	}
}

// finish recompresses edited object streams and builds the result.
func (s *pdfStrip) finish() {
	failed := map[int]bool{}
	for i, stm := range s.stms {
		if !stm.dirty {
			continue
		}
		o := stm.obj
		packed, ok := flateExact(stm.data, o.dataEnd-o.dataStart)
		if !ok {
			failed[i+1] = true
			continue
		}
		copy(s.out[o.dataStart:o.dataEnd], packed)
	}

	for _, kind := range s.order {
		lost := false
		for buf := range s.bufOf[kind] {
			lost = lost || failed[buf]
		}
		if lost {
			s.res.Kept = append(s.res.Kept, Item{"Not covered", kind + " inside a compressed object stream that could not be rewritten"})
			continue
		}
		s.res.Removed = append(s.res.Removed, Item{kind, summary(capDetails(dedupe(s.removed[kind])))})
	}
	s.res.Data = s.out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// flateExact zlib-compresses data into exactly size bytes. The deflate
// stream is ended with stored blocks of spaces, which pad the decompressed
// object stream harmlessly after its last object.
func flateExact(data []byte, size int) ([]byte, bool) {
	var buf bytes.Buffer
	buf.Write([]byte{0x78, 0xDA})
	fw, _ := flate.NewWriter(&buf, flate.BestCompression)
	fw.Write(data)
	fw.Flush() // byte-aligned sync point, stream not finished

	remaining := size - buf.Len() - 4 // bytes left for stored blocks
	if remaining < 5 {
		return nil, false
	}
	sum := adler32.New()
	sum.Write(data)

	for remaining > 0 {
		n := min(remaining-5, 0xFFFF)
		final := remaining-5 == n
		hdr := byte(0)
		if final {
			hdr = 1
		}
		buf.WriteByte(hdr)
		buf.Write(binary.LittleEndian.AppendUint16(nil, uint16(n)))
		buf.Write(binary.LittleEndian.AppendUint16(nil, ^uint16(n)))
		pad := spaces(n)
		buf.Write(pad)
		sum.Write(pad)
		remaining -= 5 + n
		if !final && remaining < 5 {
			return nil, false
		}
	}
	buf.Write(binary.BigEndian.AppendUint32(nil, sum.Sum32()))
	return buf.Bytes(), buf.Len() == size
}

// decodePDFString decodes a (literal) or <hex> string token for display.
func decodePDFString(tok []byte) string {
	var raw []byte
	switch {
	case len(tok) >= 2 && tok[0] == '<':
		hex := make([]byte, 0, len(tok))
		for _, c := range tok[1 : len(tok)-1] {
			if !isPDFSpace(c) {
				hex = append(hex, c)
			}
		}
		if len(hex)%2 == 1 {
			hex = append(hex, '0')
		}
		for i := 0; i+1 < len(hex); i += 2 {
			v, err := strconv.ParseUint(string(hex[i:i+2]), 16, 8)
			if err != nil {
				return ""
			}
			raw = append(raw, byte(v))
		}
	case len(tok) >= 2 && tok[0] == '(':
		in := tok[1 : len(tok)-1]
		for i := 0; i < len(in); i++ {
			c := in[i]
			if c != '\\' || i+1 >= len(in) {
				raw = append(raw, c)
				continue
			}
			i++
			switch e := in[i]; e {
			case 'n':
				raw = append(raw, '\n')
			case 'r':
				raw = append(raw, '\r')
			case 't':
				raw = append(raw, '\t')
			case 'b', 'f':
			case '\r', '\n':
			default:
				if e >= '0' && e <= '7' {
					v := 0
					for k := 0; k < 3 && i < len(in) && in[i] >= '0' && in[i] <= '7'; k++ {
						v = v*8 + int(in[i]-'0')
						i++
					}
					i--
					raw = append(raw, byte(v))
				} else {
					raw = append(raw, e)
				}
			}
		}
	}

	if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
		u := make([]uint16, 0, len(raw)/2)
		for i := 2; i+1 < len(raw); i += 2 {
			u = append(u, uint16(raw[i])<<8|uint16(raw[i+1]))
		}
		return strings.TrimSpace(string(utf16.Decode(u)))
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(raw), ""))
}

// pdfDate turns "D:20240501102201+03'00'" into "2024-05-01 10:22:01".
func pdfDate(s string) string {
	if !strings.HasPrefix(s, "D:") || len(s) < 16 || !isInt(s[2:10]) {
		return s
	}
	return fmt.Sprintf("%s-%s-%s %s:%s:%s", s[2:6], s[6:8], s[8:10], s[10:12], s[12:14], s[14:16])
}
