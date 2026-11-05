package vault

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"testing"
	"time"
)

const testIter = 1000

func seal(t *testing.T, pass string, data []byte, md *Metadata) []byte {
	return sealOpts(t, pass, data, md, Options{})
}

func sealOpts(t *testing.T, pass string, data []byte, md *Metadata, opts Options) []byte {
	t.Helper()
	k, err := NewSealKey([]byte(pass), testIter)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w, err := NewWriter(&buf, k, md, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Write in odd-sized pieces to exercise buffering.
	for p := data; len(p) > 0; {
		n := min(len(p), 7777)
		if _, err := w.Write(p[:n]); err != nil {
			t.Fatal(err)
		}
		p = p[n:]
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func open(pass string, sealed []byte) ([]byte, *Reader, error) {
	r, err := NewReader(bytes.NewReader(sealed), NewPassphrase([]byte(pass)))
	if err != nil {
		return nil, nil, err
	}
	out, err := io.ReadAll(r)
	return out, r, err
}

func TestRoundTripSizes(t *testing.T) {
	chunk := 1 << DefaultChunkShift
	for _, size := range []int{0, 1, 100, chunk - 1, chunk, chunk + 1, 3 * chunk, 3*chunk + 5} {
		data := make([]byte, size)
		rand.Read(data)
		for _, md := range []*Metadata{nil, {Name: "a.txt", Size: int64(size), Mode: 0o600, ModTime: time.Unix(1700000000, 0).UTC()}} {
			for _, opts := range []Options{{}, {Compress: true}} {
				sealed := sealOpts(t, "pw", data, md, opts)
				got, r, err := open("pw", sealed)
				if err != nil {
					t.Fatalf("size %d md %v: %v", size, md != nil, err)
				}
				if !bytes.Equal(got, data) {
					t.Fatalf("size %d: plaintext mismatch", size)
				}
				if md != nil && *r.Metadata != *md {
					t.Fatalf("metadata mismatch: %+v vs %+v", r.Metadata, md)
				}
				if md == nil && r.Metadata != nil {
					t.Fatal("unexpected metadata")
				}
				if r.Header.Compressed() != opts.Compress {
					t.Fatal("compression flag mismatch")
				}
			}
		}
	}
}

func TestWrongKey(t *testing.T) {
	sealed := seal(t, "right", []byte("hello"), nil)
	if _, _, err := open("wrong", sealed); !errors.Is(err, ErrAuth) {
		t.Fatalf("want ErrAuth, got %v", err)
	}
}

func TestTamperEveryRegion(t *testing.T) {
	data := make([]byte, 2*(1<<DefaultChunkShift)+10)
	rand.Read(data)
	sealed := seal(t, "pw", data, &Metadata{Name: "x"})

	for _, off := range []int{5, 9, 20, 40, HeaderSize, HeaderSize + 1000, len(sealed) / 2, len(sealed) - 1} {
		bad := bytes.Clone(sealed)
		bad[off] ^= 0x01
		if _, _, err := open("pw", bad); err == nil {
			t.Fatalf("tamper at %d not detected", off)
		}
	}
}

func TestTruncation(t *testing.T) {
	chunk := 1 << DefaultChunkShift
	data := make([]byte, 3*chunk)
	rand.Read(data)
	sealed := seal(t, "pw", data, nil)

	// Cut exactly at chunk boundaries (the dangerous case) and mid-chunk.
	for _, cut := range []int{HeaderSize, HeaderSize + chunk + tagSize, HeaderSize + 2*(chunk+tagSize), len(sealed) - 1, HeaderSize + 10} {
		if _, _, err := open("pw", sealed[:cut]); err == nil {
			t.Fatalf("truncation at %d not detected", cut)
		}
	}
}

func TestTrailingGarbage(t *testing.T) {
	sealed := seal(t, "pw", []byte("hello"), nil)
	if _, _, err := open("pw", append(bytes.Clone(sealed), 0)); err == nil {
		t.Fatal("trailing data not detected")
	}
}

func TestChunkReorder(t *testing.T) {
	chunk := 1 << DefaultChunkShift
	data := make([]byte, 3*chunk)
	rand.Read(data)
	sealed := seal(t, "pw", data, nil)

	c := chunk + tagSize
	body := sealed[HeaderSize:]
	swapped := append(bytes.Clone(sealed[:HeaderSize]), body[c:2*c]...)
	swapped = append(swapped, body[:c]...)
	swapped = append(swapped, body[2*c:]...)
	if _, _, err := open("pw", swapped); !errors.Is(err, ErrAuth) {
		t.Fatalf("reorder not detected: %v", err)
	}
}

func TestNotSealed(t *testing.T) {
	for _, in := range [][]byte{nil, []byte("abc"), []byte("plain text that is long enough to fill a whole header......")} {
		if _, err := NewReader(bytes.NewReader(in), NewPassphrase([]byte("pw"))); !errors.Is(err, ErrNotSealed) {
			t.Fatalf("%q: want ErrNotSealed, got %v", in, err)
		}
	}
}

func TestSharedKeyUniqueFiles(t *testing.T) {
	k, _ := NewSealKey([]byte("pw"), testIter)
	var a, b bytes.Buffer
	for _, buf := range []*bytes.Buffer{&a, &b} {
		w, _ := NewWriter(buf, k, nil, Options{})
		w.Write([]byte("same"))
		w.Close()
	}
	if bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("two files with the same key and plaintext must differ")
	}
	if !bytes.Equal(a.Bytes()[12:28], b.Bytes()[12:28]) {
		t.Fatal("files sealed with one SealKey should share the KDF salt")
	}
}

func TestCompression(t *testing.T) {
	text := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog\n"), 20000)

	plain := seal(t, "pw", text, nil)
	packed := sealOpts(t, "pw", text, nil, Options{Compress: true})
	if len(packed) >= len(plain)/10 {
		t.Fatalf("compression ineffective: %d vs %d bytes", len(packed), len(plain))
	}

	got, _, err := open("pw", packed)
	if err != nil || !bytes.Equal(got, text) {
		t.Fatalf("round trip failed: %v", err)
	}

	bad := bytes.Clone(packed)
	bad[len(bad)-20] ^= 1
	if _, _, err := open("pw", bad); err == nil {
		t.Fatal("tamper not detected with compression")
	}

	// Clearing the gzip flag in the header must not go unnoticed (header is AAD).
	bad = bytes.Clone(packed)
	bad[5] &^= FlagGzip
	if _, _, err := open("pw", bad); err == nil {
		t.Fatal("header flag change not detected")
	}
}
