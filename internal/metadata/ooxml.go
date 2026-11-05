package metadata

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// Office Open XML documents (DOCX, XLSX, PPTX) are zip packages. Their
// metadata lives in docProps/: core.xml (author, last modified by, dates,
// title, keywords...), app.xml (company, manager, template, editing time,
// application) and custom.xml (custom properties). These parts are replaced
// with empty, valid documents (so relationships keep pointing at existing
// parts), embedded photos (*/media/*) are stripped like standalone images
// and every zip entry gets the same fixed timestamp.

const (
	emptyCore = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"></cp:coreProperties>`
	emptyApp = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"></Properties>`
	emptyCustom = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/custom-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"></Properties>`
)

// zipEpoch is the earliest time a zip entry can store.
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// Fields of app.xml worth reporting; the rest are statistics (pages, words).
var appFields = map[string]bool{
	"Company": true, "Manager": true, "Template": true, "TotalTime": true,
	"Application": true, "AppVersion": true, "HyperlinkBase": true,
}

// Parts that still carry author names after stripping.
var authorParts = []string{
	"word/comments.xml", "word/people.xml", "word/commentsExtended.xml",
	"ppt/commentAuthors.xml", "xl/persons/person.xml",
}

func stripOOXML(b []byte, _ Options) (*Result, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, ErrUnsupported
	}

	names := map[string]*zip.File{}
	for _, f := range zr.File {
		names[f.Name] = f
	}
	if names["[Content_Types].xml"] == nil {
		return nil, ErrUnsupported // a plain zip, not an Office document
	}

	res := &Result{Format: "Office"}
	switch {
	case names["word/document.xml"] != nil:
		res.Format = "DOCX"
	case names["xl/workbook.xml"] != nil:
		res.Format = "XLSX"
	case names["ppt/presentation.xml"] != nil:
		res.Format = "PPTX"
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	timestamps := false
	var images []string

	for _, f := range zr.File {
		if !f.Modified.IsZero() && !f.Modified.Equal(zipEpoch) {
			timestamps = true
		}

		replacement := ""
		switch f.Name {
		case "docProps/core.xml":
			replacement = emptyCore
		case "docProps/app.xml":
			replacement = emptyApp
		case "docProps/custom.xml":
			replacement = emptyCustom
		}

		if replacement != "" {
			items, err := xmlFields(f, f.Name == "docProps/app.xml")
			if err != nil {
				return nil, err
			}
			if f.Name == "docProps/custom.xml" && len(items) > 0 {
				res.Removed = append(res.Removed, Item{"Custom properties", strings.Join(items, ", ")})
			} else {
				for _, it := range items {
					res.Removed = append(res.Removed, Item{"Property", it})
				}
			}

			w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: zipEpoch})
			if err != nil {
				return nil, err
			}
			if _, err := io.WriteString(w, replacement); err != nil {
				return nil, err
			}
			continue
		}

		// Photos pasted into documents keep their EXIF (GPS, camera, ...).
		if strings.Contains(f.Name, "/media/") {
			cleaned, items, err := cleanMedia(f)
			if err != nil {
				return nil, err
			}
			if cleaned != nil {
				var parts []string
				for _, it := range items {
					if it.Kind == "EXIF" && it.Detail != "" {
						parts = append(parts, "EXIF "+it.Detail)
					} else {
						parts = append(parts, it.Kind)
					}
				}
				images = append(images, f.Name[strings.LastIndex(f.Name, "/")+1:]+": "+strings.Join(parts, ", "))
				w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: f.Method, Modified: zipEpoch})
				if err != nil {
					return nil, err
				}
				if _, err := w.Write(cleaned); err != nil {
					return nil, err
				}
				continue
			}
		}

		// Copy the compressed bytes as-is, only normalising the header.
		fh := f.FileHeader
		fh.Modified = zipEpoch
		fh.ModifiedTime, fh.ModifiedDate = 0, 0x21 // 1980-01-01 00:00 in MS-DOS format (CreateRaw ignores Modified)
		fh.Extra = nil
		fh.Comment = ""
		raw, err := f.OpenRaw()
		if err != nil {
			return nil, err
		}
		w, err := zw.CreateRaw(&fh)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(w, raw); err != nil {
			return nil, err
		}
	}

	if len(images) > 0 {
		res.Removed = append(res.Removed, Item{"Embedded images", summary(capDetails(images))})
	}
	if zr.Comment != "" {
		res.Removed = append(res.Removed, Item{"Zip comment", truncate(zr.Comment, 50)})
	}
	if timestamps {
		res.Removed = append(res.Removed, Item{"Timestamps", "zip entry modification times"})
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	for _, p := range authorParts {
		if names[p] != nil {
			res.Kept = append(res.Kept, Item{"Not covered", "comments / tracked changes still show author names"})
			break
		}
	}

	res.Data = out.Bytes()
	return res, nil
}

// xmlFields lists "name: value" for the non-empty leaf elements of a
// properties part. For app.xml only identifying fields are listed.
func xmlFields(f *zip.File, onlyIdentifying bool) ([]string, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var (
		items []string
		stack []string
		text  strings.Builder
		name  string // custom.xml: <property name="...">
	)
	dec := xml.NewDecoder(io.LimitReader(rc, 4<<20))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return items, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			text.Reset()
			if t.Name.Local == "property" {
				for _, a := range t.Attr {
					if a.Name.Local == "name" {
						name = a.Value
					}
				}
			}
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			v := strings.TrimSpace(text.String())
			text.Reset()
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			switch {
			case t.Name.Local == "property" && name != "":
				items = append(items, name)
				name = ""
			case v == "" || len(stack) != 1: // only direct children of the root
			case onlyIdentifying && !appFields[t.Name.Local]:
			default:
				items = append(items, t.Name.Local+": "+truncate(v, 50))
			}
		}
	}
}

// cleanMedia strips an embedded image. It returns nil if the entry is not
// a supported image or holds no metadata.
func cleanMedia(f *zip.File) ([]byte, []Item, error) {
	if f.UncompressedSize64 > 256<<20 {
		return nil, nil, nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, err
	}
	var strip func([]byte, Options) (*Result, error)
	switch {
	case isJPEG(data):
		strip = stripJPEG
	case isPNG(data):
		strip = stripPNG
	case isWebP(data):
		strip = stripWebP
	default:
		return nil, nil, nil
	}
	res, err := strip(data, Options{KeepOrientation: true})
	if err != nil || !res.Changed() {
		return nil, nil, nil // leave unparseable images untouched
	}
	return res.Data, res.Removed, nil
}
