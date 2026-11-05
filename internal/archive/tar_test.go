package archive

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type entry struct {
	name, link, body string
	typ              byte
}

func makeTar(t *testing.T, entries ...entry) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Linkname: e.link, Typeflag: e.typ, Mode: 0o644, Size: int64(len(e.body))}
		if e.typ != tar.TypeReg {
			h.Size = 0
		}
		if e.typ == tar.TypeDir {
			h.Mode = 0o755
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(e.body))
	}
	tw.Close()
	return &buf
}

func TestExtractRejectsEscapes(t *testing.T) {
	cases := map[string][]entry{
		"dotdot path":   {{name: "../evil", body: "x", typ: tar.TypeReg}},
		"absolute path": {{name: "/tmp/evil", body: "x", typ: tar.TypeReg}},
		"absolute link": {{name: "l", link: "/etc", typ: tar.TypeSymlink}},
		"climbing link": {{name: "l", link: "..", typ: tar.TypeSymlink}},
		"deep climb":    {{name: "a/l", link: "../../x", typ: tar.TypeSymlink}},
		"mid dotdot":    {{name: "l", link: "a/../../x", typ: tar.TypeSymlink}},
		// A link to "." makes "y/.." point outside if checked lexically only.
		"link to self then climb": {
			{name: "y", link: ".", typ: tar.TypeSymlink},
			{name: "y/l", link: "..", typ: tar.TypeSymlink},
		},
		// Writing through a symlink must not escape (os.Root blocks it).
		"write through link": {
			{name: "d", typ: tar.TypeDir},
			{name: "d/l", link: "..", typ: tar.TypeSymlink},
			{name: "d/l/l2", link: "..", typ: tar.TypeSymlink},
		},
	}

	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			out := filepath.Join(parent, "out")

			_ = ExtractTar(makeTar(t, entries...), out, ExtractOptions{})

			// Nothing may appear next to the output directory.
			des, _ := os.ReadDir(parent)
			for _, d := range des {
				if d.Name() != "out" {
					t.Fatalf("escaped: created %s outside output", d.Name())
				}
			}
			// And no symlink inside may resolve outside it.
			filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
				if err == nil && info.Mode()&os.ModeSymlink != 0 {
					real, err := filepath.EvalSymlinks(p)
					outReal, _ := filepath.EvalSymlinks(out)
					if err == nil && !strings.HasPrefix(real, outReal) {
						t.Fatalf("symlink %s resolves outside: %s", p, real)
					}
				}
				return nil
			})
		})
	}
}

func TestExtractAllowsInternalLinks(t *testing.T) {
	out := t.TempDir()
	err := ExtractTar(makeTar(t,
		entry{name: "a.txt", body: "hi", typ: tar.TypeReg},
		entry{name: "sub/", typ: tar.TypeDir},
		entry{name: "sub/up", link: "../a.txt", typ: tar.TypeSymlink},
		entry{name: "here", link: "sub", typ: tar.TypeSymlink},
	), out, ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "sub", "up"))
	if err != nil || string(b) != "hi" {
		t.Fatalf("link not usable: %q %v", b, err)
	}
}

func TestExtractForce(t *testing.T) {
	out := t.TempDir()
	tarball := func() *bytes.Buffer { return makeTar(t, entry{name: "f", body: "new", typ: tar.TypeReg}) }

	os.WriteFile(filepath.Join(out, "f"), []byte("old"), 0o644)
	if err := ExtractTar(tarball(), out, ExtractOptions{}); err == nil {
		t.Fatal("expected error without force")
	}
	if err := ExtractTar(tarball(), out, ExtractOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "f")); string(b) != "new" {
		t.Fatalf("got %q", b)
	}
}
