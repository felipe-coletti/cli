package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func entries(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range des {
		names = append(names, d.Name())
	}
	return names
}

func TestAtomicCommit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.bin")

	a, err := CreateAtomic(path, false, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	a.Write([]byte("data"))

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("target must not exist before Commit")
	}
	if err := a.Commit(); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "data" {
		t.Fatalf("got %q", got)
	}
	if n := entries(t, dir); len(n) != 1 {
		t.Fatalf("leftover files: %v", n)
	}
}

func TestAtomicAbortLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	a, err := CreateAtomic(filepath.Join(dir, "out.bin"), false, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	a.Write([]byte("partial"))
	a.Abort()

	if n := entries(t, dir); len(n) != 0 {
		t.Fatalf("leftover files: %v", n)
	}
}

func TestAtomicNoClobber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.bin")
	os.WriteFile(path, []byte("old"), 0o644)

	if _, err := CreateAtomic(path, false, 0o644); err == nil {
		t.Fatal("expected error for existing file without force")
	}

	// File appears between the existence check and Commit: must not be overwritten.
	os.Remove(path)
	a, _ := CreateAtomic(path, false, 0o644)
	a.Write([]byte("new"))
	os.WriteFile(path, []byte("raced"), 0o644)
	if err := a.Commit(); err == nil {
		t.Fatal("expected Commit to refuse clobbering")
	}
	if got, _ := os.ReadFile(path); string(got) != "raced" {
		t.Fatalf("file was clobbered: %q", got)
	}

	a, _ = CreateAtomic(path, true, 0o644)
	a.Write([]byte("forced"))
	if err := a.Commit(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "forced" {
		t.Fatalf("got %q", got)
	}
}
