package crypto

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"cli/internal/cli"
	"cli/internal/meta"
	"cli/internal/vault"
)

// ext is the extension of sealed files: the CLI name, e.g. ".mycli".
// It follows meta.Name, so renaming the CLI renames the extension too.
func ext() string { return "." + meta.Name }

// keyEnv is the environment variable read when no key flag is given.
func keyEnv() string { return strings.ToUpper(meta.Name) + "_KEY" }

// ---------------------------------------------------------------------------
// Key input

type keyOpts struct {
	key     string
	keyFile string
}

func (k *keyOpts) register(fs *cli.FlagSet) {
	fs.StringVar(&k.key, "key", "k", "", "key", "Key / passphrase (visible in shell history; prefer the prompt or --key-file)")
	fs.StringVar(&k.keyFile, "key-file", "", "", "path", fmt.Sprintf("Read the key from a file\nWithout --key or --key-file: $%s, then an interactive prompt", keyEnv()))
}

// given returns the key from --key, --key-file or the environment, or nil
// if none was provided.
func (k *keyOpts) given() ([]byte, error) {
	switch {
	case k.key != "":
		return []byte(k.key), nil
	case k.keyFile != "":
		return readKeyFile(k.keyFile)
	case os.Getenv(keyEnv()) != "":
		return []byte(os.Getenv(keyEnv())), nil
	}
	return nil, nil
}

// resolve returns the key from flags/environment or asks for it on the
// terminal. confirm asks twice (used when encrypting, where a typo would
// make the data unrecoverable).
func (k *keyOpts) resolve(confirm bool) ([]byte, error) {
	key, err := k.given()
	if err != nil || key != nil {
		return key, err
	}

	key, err = cli.ReadPassword("Passphrase: ")
	if errors.Is(err, cli.ErrNoTerminal) {
		return nil, cli.Usagef("a key is required: use --key, --key-file or $%s", keyEnv())
	}
	if err != nil {
		return nil, err
	}
	if len(key) == 0 {
		return nil, errors.New("empty passphrase")
	}

	if confirm {
		again, err := cli.ReadPassword("Confirm passphrase: ")
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(key, again) {
			return nil, errors.New("passphrases do not match")
		}
	}

	return key, nil
}

func readKeyFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	defer f.Close()

	// Like ssh with private keys: complain if others can read the key.
	if fi, err := f.Stat(); err == nil && runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "warning: key file %s is accessible by other users (mode %04o); run: chmod 600 %s\n",
			path, fi.Mode().Perm(), path)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	data = bytes.TrimRight(data, "\r\n")
	if len(data) == 0 {
		return nil, errors.New("key file is empty")
	}
	return data, nil
}

func isDir(path string) bool {
	if path == "-" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// ---------------------------------------------------------------------------
// Single file / stream

// sealStream encrypts in → outPath ("-" = stdout). When stdout is a terminal
// the binary output is base64-encoded so it can be copied and pasted.
func sealStream(in io.Reader, md *vault.Metadata, outPath string, key *vault.SealKey, opts vault.Options, force bool) error {
	out, err := cli.CreateOutput(outPath, force)
	if err != nil {
		return err
	}

	err = func() error {
		var dst io.Writer = out
		var b64 io.WriteCloser
		if outPath == "-" && !cli.IsOutputPiped() {
			b64 = base64.NewEncoder(base64.StdEncoding, out)
			dst = b64
		}

		w, err := vault.NewWriter(dst, key, md, opts)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, in); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		if b64 != nil {
			if err := b64.Close(); err != nil {
				return err
			}
			fmt.Fprintln(out)
		}
		return out.Commit()
	}()

	if err != nil {
		out.Abort()
	}
	return err
}

func sealFile(inPath, outPath string, key *vault.SealKey, opts vault.Options, force bool) error {
	f, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file", inPath)
	}

	return sealStream(f, vault.MetadataFromFileInfo(fi), outPath, key, opts, force)
}

// sniff wraps r so that both raw sealed data and its base64 form are accepted.
// It reports whether the data is in the sealed format at all.
func sniff(r io.Reader) (io.Reader, bool) {
	br := bufio.NewReader(r)
	head, _ := br.Peek(8)

	if vault.IsSealed(head) {
		return br, true
	}

	if len(head) == 8 {
		if dec, err := base64.StdEncoding.DecodeString(string(head)); err == nil && vault.IsSealed(dec) {
			return base64.NewDecoder(base64.StdEncoding, br), true
		}
	}

	return br, false
}

// openStream decrypts a sealed stream. If outPath is an existing directory
// the original file name stored in the encrypted metadata is used.
// It returns the path written to.
func openStream(in io.Reader, outPath string, keys *vault.Passphrase, force bool) (string, error) {
	r, err := vault.NewReader(in, keys)
	if err != nil {
		return "", err
	}

	if isDir(outPath) {
		name := ""
		if r.Metadata != nil {
			name = filepath.Base(filepath.Clean("/" + r.Metadata.Name))
		}
		if name == "" || name == "/" || name == "." {
			return "", errors.New("output is a directory but the file has no stored name; pass a file path to --output")
		}
		outPath = filepath.Join(outPath, name)
	}

	out, err := cli.CreateOutput(outPath, force)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(out, r); err != nil {
		out.Abort()
		return "", err
	}

	if af, ok := out.(*cli.AtomicFile); ok && r.Metadata != nil && r.Metadata.Mode != 0 {
		af.File().Chmod(r.Metadata.Mode.Perm())
	}

	if err := out.Commit(); err != nil {
		return "", err
	}

	if outPath != "-" && r.Metadata != nil && !r.Metadata.ModTime.IsZero() {
		os.Chtimes(outPath, r.Metadata.ModTime, r.Metadata.ModTime)
	}

	return outPath, nil
}

func openFile(inPath, outPath string, keys *vault.Passphrase, force bool) error {
	f, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = openStream(f, outPath, keys, force)
	return err
}
