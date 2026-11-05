package crypto

import (
	"cli/internal/cli"
	"cli/internal/crypto"
	"cli/internal/meta"
	"cli/internal/vault"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type encryptOpts struct {
	cipher     string
	keys       keyOpts
	input      string
	output     string
	force      bool
	jobs       int
	iterations int
	compress   bool
}

func NewEncryptCmd() *cli.Command {
	o := &encryptOpts{}
	n := meta.Name

	return &cli.Command{
		Name:        "encrypt",
		Aliases:     []string{"enc"},
		Description: "Encrypt a message, file or directory",
		Long: `
With the default aes cipher the output uses the sealed v1 format:
PBKDF2-SHA256 key derivation, AES-256-GCM in authenticated 64 KiB chunks,
streaming (constant memory) and atomic writes.`,
		Flags: func(fs *cli.FlagSet) {
			fs.StringVar(&o.cipher, "cipher", "c", "aes", "type", "Cipher: aes, xor, rot13, caesar, vigenere")
			o.keys.register(fs)
			fs.StringVar(&o.input, "input", "i", "-", "path", "Input file or directory (default: stdin)")
			fs.StringVar(&o.output, "output", "o", "-", "path", "Output file, or directory for directory input (default: stdout)")
			fs.BoolVar(&o.force, "force", "F", false, "Overwrite output files if they exist")
			fs.BoolVar(&o.compress, "compress", "z", false, "Compress with gzip before encrypting (text, logs; not photos/video)")
			fs.IntVar(&o.jobs, "jobs", "j", 0, "n", "Parallel workers in directory mode (default: CPU count)")
			fs.IntVar(&o.iterations, "iterations", "", vault.DefaultIterations, "n", "PBKDF2 iterations")
		},
		Examples: []string{
			fmt.Sprintf(`echo "secret" | %s crypto encrypt`, n),
			fmt.Sprintf("%s crypto encrypt --key-file ~/.key -i notes.txt -o notes.txt%s", n, ext()),
			fmt.Sprintf("%s crypto encrypt --key-file ~/.key -i ./photos -o ./photos.sealed -j 8", n),
			fmt.Sprintf("%s archive pack -i ./docs | %s crypto encrypt -z -o docs.tar%s", n, n, ext()),
		},
		Run: func(args []string) error { return o.run() },
	}
}

func (o *encryptOpts) run() error {
	key, err := o.keys.resolve(true)
	if err != nil {
		return err
	}

	if strings.ToLower(o.cipher) != "aes" {
		if isDir(o.input) {
			return cli.Usage(errors.New("directory mode is only supported with the aes cipher"))
		}
		return classicEncrypt(o.cipher, key, o.input, o.output, o.force)
	}

	sk, err := vault.NewSealKey(key, o.iterations)
	if err != nil {
		return err
	}
	opts := vault.Options{Compress: o.compress}

	switch {
	case isDir(o.input):
		if o.output == "-" {
			return cli.Usage(errors.New("--output (-o) must be a directory when the input is a directory"))
		}
		return encryptTree(o.input, o.output, sk, opts, o.force, o.jobs)
	case o.input == "-":
		return sealStream(os.Stdin, nil, o.output, sk, opts, o.force)
	default:
		return sealFile(o.input, o.output, sk, opts, o.force)
	}
}

func encryptTree(src, dst string, sk *vault.SealKey, opts vault.Options, force bool, workers int) error {
	jobs, err := cli.PlanTree(src, dst, func(rel string) (string, bool) {
		return rel + ext(), true
	})
	if err != nil {
		return err
	}

	errs := cli.RunPool(jobs, workers, func(j cli.Job) error {
		if err := cli.MkParent(j.Out); err != nil {
			return err
		}
		return sealFile(j.In, j.Out, sk, opts, force)
	})

	return cli.ReportTree("encrypted", len(jobs), errs)
}

// classicEncrypt handles the classic (non-AES) ciphers, which work in memory.
func classicEncrypt(cipher string, key []byte, input, output string, force bool) error {
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

	result, err := crypto.Encrypt(cipher, string(key), data)
	if err != nil {
		return err
	}

	if output == "-" && !cli.IsOutputPiped() {
		fmt.Println(base64.StdEncoding.EncodeToString(result))
		return nil
	}

	out, err := cli.CreateOutput(output, force)
	if err != nil {
		return err
	}
	if _, err := out.Write(result); err != nil {
		out.Abort()
		return fmt.Errorf("writing output: %w", err)
	}
	return out.Commit()
}
