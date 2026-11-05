package crypto

import (
	"cli/internal/cli"
	"cli/internal/crypto"
	"cli/internal/meta"
	"cli/internal/vault"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type decryptOpts struct {
	cipher string
	keys   keyOpts
	input  string
	output string
	force  bool
	jobs   int
}

func NewDecryptCmd() *cli.Command {
	o := &decryptOpts{}
	n := meta.Name

	return &cli.Command{
		Name:        "decrypt",
		Aliases:     []string{"dec"},
		Description: "Decrypt a message, file or directory",
		Long:        "Sealed input may be raw or base64 (as printed to a terminal by encrypt).",
		Flags: func(fs *cli.FlagSet) {
			fs.StringVar(&o.cipher, "cipher", "c", "aes", "type", "Cipher: aes, xor, rot13, caesar, vigenere")
			o.keys.register(fs)
			fs.StringVar(&o.input, "input", "i", "-", "path", "Input file or directory (default: stdin)")
			fs.StringVar(&o.output, "output", "o", "-", "path", "Output file or directory (default: stdout)\nAn existing directory restores the original file name")
			fs.BoolVar(&o.force, "force", "F", false, "Overwrite output files if they exist")
			fs.IntVar(&o.jobs, "jobs", "j", 0, "n", "Parallel workers in directory mode (default: CPU count)")
		},
		Examples: []string{
			fmt.Sprintf(`echo "<base64>" | %s crypto decrypt`, n),
			fmt.Sprintf("%s crypto decrypt --key-file ~/.key -i notes.txt%s -o .", n, ext()),
			fmt.Sprintf("%s crypto decrypt --key-file ~/.key -i ./photos.sealed -o ./photos", n),
		},
		Run: func(args []string) error { return o.run() },
	}
}

func (o *decryptOpts) run() error {
	key, err := o.keys.resolve(false)
	if err != nil {
		return err
	}

	if strings.ToLower(o.cipher) != "aes" {
		if isDir(o.input) {
			return cli.Usage(errors.New("directory mode is only supported with the aes cipher"))
		}
		return classicDecrypt(o.cipher, key, o.input, o.output, o.force)
	}

	pass := vault.NewPassphrase(key)

	if isDir(o.input) {
		if o.output == "-" {
			return cli.Usage(errors.New("--output (-o) must be a directory when the input is a directory"))
		}
		return decryptTree(o.input, o.output, pass, o.force, o.jobs)
	}

	in, err := cli.OpenInput(o.input)
	if err != nil {
		return err
	}
	defer in.Close()

	r, sealed := sniff(in)
	if !sealed {
		return vault.ErrNotSealed
	}

	written, err := openStream(r, o.output, pass, o.force)
	if err != nil {
		return err
	}
	if written != "-" && o.output != written {
		fmt.Fprintf(os.Stderr, "wrote %s\n", written)
	}
	return nil
}

func decryptTree(src, dst string, pass *vault.Passphrase, force bool, workers int) error {
	jobs, err := cli.PlanTree(src, dst, func(rel string) (string, bool) {
		if !strings.HasSuffix(rel, ext()) || len(rel) == len(ext()) {
			return "", false
		}
		return strings.TrimSuffix(rel, ext()), true
	})
	if err != nil {
		return err
	}

	errs := cli.RunPool(jobs, workers, func(j cli.Job) error {
		if err := cli.MkParent(j.Out); err != nil {
			return err
		}
		return openFile(j.In, j.Out, pass, force)
	})

	return cli.ReportTree("decrypted", len(jobs), errs)
}

// classicDecrypt handles the classic (non-AES) ciphers, which work in memory.
func classicDecrypt(cipher string, key []byte, input, output string, force bool) error {
	in, err := cli.OpenInput(input)
	if err != nil {
		return err
	}
	defer in.Close()

	data, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	if len(data) == 0 {
		return errors.New("input is empty")
	}

	result, err := crypto.Decrypt(cipher, string(key), data)
	if err != nil {
		return err
	}

	out, err := cli.CreateOutput(output, force)
	if err != nil {
		return err
	}
	if _, err := out.Write(result); err != nil {
		out.Abort()
		return fmt.Errorf("writing output: %w", err)
	}
	if err := out.Commit(); err != nil {
		return err
	}

	if output == "-" && !cli.IsOutputPiped() {
		fmt.Println()
	}
	return nil
}
