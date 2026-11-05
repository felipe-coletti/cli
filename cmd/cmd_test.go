package cmd

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cli/internal/cli"
	"cli/internal/meta"
)

// run executes the CLI in-process with stdout/stderr silenced.
func run(t *testing.T, args ...string) int {
	t.Helper()
	devnull, _ := os.Open(os.DevNull)
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = devnull, devnull
	defer func() { os.Stdout, os.Stderr = stdout, stderr; devnull.Close() }()
	return Main(args)
}

func mustRun(t *testing.T, args ...string) {
	t.Helper()
	if code := run(t, args...); code != 0 {
		t.Fatalf("%v: exit %d", args, code)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

// snapshot maps relative path -> content for every regular file under dir.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if d.Type().IsRegular() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(dir, p)
			out[rel] = string(b)
		}
		return nil
	})
	return out
}

func equalTrees(t *testing.T, a, b string) {
	t.Helper()
	sa, sb := snapshot(t, a), snapshot(t, b)
	if len(sa) != len(sb) {
		t.Fatalf("trees differ: %v vs %v", sa, sb)
	}
	for k, v := range sa {
		if sb[k] != v {
			t.Fatalf("file %s differs", k)
		}
	}
}

func TestEncryptDecryptFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.txt")
	write(t, src, strings.Repeat("hello world\n", 5000))

	for _, extra := range [][]string{nil, {"--compress"}} {
		sealed := filepath.Join(dir, "notes.txt"+"."+meta.Name)
		os.Remove(sealed)

		mustRun(t, append([]string{"crypto", "encrypt", "-k", "pw", "--iterations", "1000", "-i", src, "-o", sealed}, extra...)...)

		// Decrypting into a directory restores the original name and mode.
		out := filepath.Join(dir, "out")
		os.RemoveAll(out)
		os.Mkdir(out, 0o755)
		mustRun(t, "crypto", "decrypt", "-k", "pw", "-i", sealed, "-o", out)

		got, err := os.ReadFile(filepath.Join(out, "notes.txt"))
		if err != nil {
			t.Fatal(err)
		}
		want, _ := os.ReadFile(src)
		if !bytes.Equal(got, want) {
			t.Fatal("round trip mismatch")
		}
		if fi, _ := os.Stat(filepath.Join(out, "notes.txt")); fi.Mode().Perm() != 0o640 {
			t.Fatalf("mode not restored: %v", fi.Mode())
		}

		mustRun(t, "crypto", "inspect", "-k", "pw", sealed)
	}
}

func TestWrongKeyLeavesNoOutput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	write(t, src, "secret")
	sealed := filepath.Join(dir, "a.sealed")
	mustRun(t, "crypto", "encrypt", "-k", "right", "--iterations", "1000", "-i", src, "-o", sealed)

	out := filepath.Join(dir, "a.out")
	if code := run(t, "crypto", "decrypt", "-k", "wrong", "-i", sealed, "-o", out); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if code := run(t, "crypto", "inspect", "-k", "wrong", sealed); code != 1 {
		t.Fatalf("inspect exit %d, want 1", code)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("leftover files after failed decrypt: %v", entries)
	}
}

func TestNoOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	write(t, src, "data")
	dst := filepath.Join(dir, "exists")
	write(t, dst, "keep me")

	if code := run(t, "crypto", "encrypt", "-k", "pw", "--iterations", "1000", "-i", src, "-o", dst); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if b, _ := os.ReadFile(dst); string(b) != "keep me" {
		t.Fatal("existing file was overwritten")
	}
	mustRun(t, "crypto", "encrypt", "-k", "pw", "--iterations", "1000", "-i", src, "-o", dst, "--force")
}

func TestDirectoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	write(t, filepath.Join(src, "a.txt"), "a")
	write(t, filepath.Join(src, "sub", "b.txt"), "b")
	write(t, filepath.Join(src, "sub", "deep", "c.bin"), strings.Repeat("\x00\xff", 100000))

	sealed := filepath.Join(dir, "sealed")
	mustRun(t, "crypto", "encrypt", "-k", "pw", "--iterations", "1000", "-j", "2", "-i", src, "-o", sealed)

	if _, err := os.Stat(filepath.Join(sealed, "sub", "b.txt."+meta.Name)); err != nil {
		t.Fatalf("expected sealed file with CLI extension: %v", err)
	}

	restored := filepath.Join(dir, "restored")
	mustRun(t, "crypto", "decrypt", "-k", "pw", "-i", sealed, "-o", restored)
	equalTrees(t, src, restored)

	// Output inside the input would recurse forever: refused.
	if code := run(t, "crypto", "encrypt", "-k", "pw", "-i", src, "-o", filepath.Join(src, "x")); code == 0 {
		t.Fatal("output inside input should fail")
	}
}

func TestArchivePipelineWithSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "docs")
	write(t, filepath.Join(src, "a.txt"), "a")
	write(t, filepath.Join(src, "sub", "b.txt"), "b")
	if err := os.Symlink("../a.txt", filepath.Join(src, "sub", "link")); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	tarPath := filepath.Join(dir, "docs.tar")
	mustRun(t, "archive", "pack", "-i", src, "-o", tarPath)

	out := filepath.Join(dir, "out")
	mustRun(t, "archive", "extract", "-i", tarPath, "-o", out)

	equalTrees(t, src, filepath.Join(out, "docs"))
	if target, err := os.Readlink(filepath.Join(out, "docs", "sub", "link")); err != nil || target != "../a.txt" {
		t.Fatalf("symlink not restored: %q %v", target, err)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"nope"},
		{"crypto", "nope"},
		{"password", "generate", "--bogus"},
		{"password", "generate", "-bogus"},
		{"archive", "pack"},
		{"help", "nope"},
	} {
		if code := run(t, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}

	for _, args := range [][]string{
		nil, {"help"}, {"crypto"}, {"help", "crypto", "encrypt"}, {"crypto", "encrypt", "--help"}, {"version"},
	} {
		if code := run(t, args...); code != 0 {
			t.Errorf("%v: exit %d, want 0", args, code)
		}
	}
}

// Every command must render its help page and have a description.
func TestAllHelpPages(t *testing.T) {
	var walk func(path []string, cmds []*cli.Command)
	walk = func(path []string, cmds []*cli.Command) {
		for _, c := range cmds {
			if c.Description == "" {
				t.Errorf("%v %s: missing description", path, c.Name)
			}
			p := append(append([]string{}, path...), c.Name)
			if c.Run == nil {
				cli.PrintGroupHelp(io.Discard, p, c.Subcommands)
				walk(p, c.Subcommands)
				continue
			}
			cli.PrintHelp(io.Discard, p, c)
		}
	}
	walk(nil, Commands())
}

// dirtyJPEG returns a valid JPEG carrying a comment with personal data.
func dirtyJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	clean := buf.Bytes()
	com := []byte("taken by Fulano")
	seg := append([]byte{0xFF, 0xFE}, binary.BigEndian.AppendUint16(nil, uint16(len(com)+2))...)
	out := append(append([]byte{}, clean[:2]...), append(seg, com...)...)
	return append(out, clean[2:]...)
}

func TestMetadataStripDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photos")
	write(t, filepath.Join(src, "a.jpg"), string(dirtyJPEG(t)))
	write(t, filepath.Join(src, "sub", "b.jpg"), string(dirtyJPEG(t)))
	write(t, filepath.Join(src, "notes.txt"), "not an image")

	mustRun(t, "metadata", "show", src)

	// Copy mode: unsupported files are left out unless asked for.
	out := filepath.Join(dir, "clean")
	mustRun(t, "metadata", "strip", "-i", src, "-o", out, "-j", "2")
	for _, name := range []string{"a.jpg", filepath.Join("sub", "b.jpg")} {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("Fulano")) {
			t.Fatalf("%s still has metadata", name)
		}
		if _, err := jpeg.Decode(bytes.NewReader(b)); err != nil {
			t.Fatalf("%s no longer decodes: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("unsupported file copied without --copy-unsupported")
	}
	if b, _ := os.ReadFile(filepath.Join(src, "a.jpg")); !bytes.Contains(b, []byte("Fulano")) {
		t.Fatal("copy mode must not touch the originals")
	}

	out2 := filepath.Join(dir, "clean2")
	mustRun(t, "metadata", "strip", "-i", src, "-o", out2, "--copy-unsupported")
	if _, err := os.Stat(filepath.Join(out2, "notes.txt")); err != nil {
		t.Fatal("--copy-unsupported did not copy")
	}

	// In place, twice: the second run must find nothing to do.
	mustRun(t, "metadata", "strip", "-i", src, "--in-place")
	before, _ := os.ReadFile(filepath.Join(src, "a.jpg"))
	if bytes.Contains(before, []byte("Fulano")) {
		t.Fatal("in-place did not clean")
	}
	mustRun(t, "metadata", "strip", "-i", src, "--in-place")
	after, _ := os.ReadFile(filepath.Join(src, "a.jpg"))
	if !bytes.Equal(before, after) {
		t.Fatal("second in-place run changed the file")
	}

	// Single unsupported file, and invalid flag combinations.
	if code := run(t, "metadata", "strip", "-i", filepath.Join(src, "notes.txt"), "-o", filepath.Join(dir, "x")); code != 1 {
		t.Fatalf("unsupported single file: exit %d", code)
	}
	for _, args := range [][]string{
		{"metadata", "strip", "-i", src},
		{"metadata", "strip", "-i", src, "-o", out, "--in-place"},
		{"metadata", "strip", "-i", src, "-o", filepath.Join(src, "inside")},
		{"metadata", "show"},
	} {
		if code := run(t, args...); code == 0 {
			t.Errorf("%v should fail", args)
		}
	}
}

func mp4Box(typ string, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	return append(append(binary.BigEndian.AppendUint32(nil, uint32(8+len(body))), typ...), body...)
}

// tinyMP4 has a movie header with a creation time and a user-data location.
func tinyMP4() []byte {
	mvhd := mp4Box("mvhd", make([]byte, 4), binary.BigEndian.AppendUint32(nil, 3797403721), binary.BigEndian.AppendUint32(nil, 3797403721), make([]byte, 88))
	loc := "+37.7749-122.4194/"
	xyz := mp4Box("\xa9xyz", binary.BigEndian.AppendUint16(nil, uint16(len(loc))), []byte{0x55, 0xc4}, []byte(loc))
	return append(append(mp4Box("ftyp", []byte("isom\x00\x00\x02\x00isom")), mp4Box("moov", mvhd, mp4Box("udta", xyz))...), mp4Box("mdat", []byte("MEDIA"))...)
}

func TestMetadataStripVideo(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "v.mp4")
	orig := tinyMP4()
	write(t, src, string(orig))

	out := filepath.Join(dir, "clean.mp4")
	mustRun(t, "metadata", "strip", "-i", src, "-o", out)
	clean, _ := os.ReadFile(out)
	if len(clean) != len(orig) || bytes.Contains(clean, []byte("37.7749")) || !bytes.HasSuffix(clean, []byte("MEDIA")) {
		t.Fatalf("bad copy-mode result: %q", clean)
	}

	mustRun(t, "metadata", "strip", "-i", src, "--in-place")
	inPlace, _ := os.ReadFile(src)
	if !bytes.Equal(inPlace, clean) {
		t.Fatal("in-place result differs from copy mode")
	}
}

func TestMetadataStripPDF(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	o1 := b.Len()
	b.WriteString("1 0 obj << /Type /Catalog >> endobj\n")
	o2 := b.Len()
	b.WriteString("2 0 obj << /Author (Fulano de Tal) /Title (Segredo) >> endobj\n")
	x := b.Len()
	b.WriteString("xref\n0 3\n0000000000 65535 f \n")
	b.WriteString(strings.Join([]string{pad10(o1), pad10(o2)}, ""))
	b.WriteString("trailer << /Size 3 /Root 1 0 R /Info 2 0 R >>\nstartxref\n" + itoa(x) + "\n%%EOF\n")
	src := filepath.Join(dir, "doc.pdf")
	write(t, src, b.String())

	mustRun(t, "metadata", "strip", "-i", src, "--in-place")
	got, _ := os.ReadFile(src)
	if len(got) != b.Len() || bytes.Contains(got, []byte("Fulano")) || bytes.Contains(got, []byte("Segredo")) {
		t.Fatalf("bad result: %q", got)
	}
}

func pad10(n int) string { return strings.Repeat("0", 10-len(itoa(n))) + itoa(n) + " 00000 n \n" }

func itoa(n int) string { return strconv.Itoa(n) }
