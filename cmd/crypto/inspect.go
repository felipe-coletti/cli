package crypto

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"cli/internal/vault"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

type inspectOpts struct {
	keys   keyOpts
	input  string
	verify bool
}

func NewInspectCmd() *cli.Command {
	o := &inspectOpts{}
	n := meta.Name

	return &cli.Command{
		Name:        "inspect",
		Aliases:     []string{"info"},
		Description: "Show a sealed file's header and verify its integrity",
		Long: `
Without a key only the plaintext header is shown. With a key (or --verify,
which prompts for one) the encrypted metadata is shown too and every chunk
is authenticated, without writing any plaintext.`,
		Args: "[file]",
		Flags: func(fs *cli.FlagSet) {
			o.keys.register(fs)
			fs.StringVar(&o.input, "input", "i", "", "path", "Sealed file (or pass it as an argument; default: stdin)")
			fs.BoolVar(&o.verify, "verify", "v", false, "Verify integrity, prompting for the key if needed")
		},
		Examples: []string{
			fmt.Sprintf("%s crypto inspect notes.txt%s", n, ext()),
			fmt.Sprintf("%s crypto inspect --verify notes.txt%s", n, ext()),
		},
		Run: o.run,
	}
}

func (o *inspectOpts) run(args []string) error {
	input := o.input
	if input == "" && len(args) > 0 {
		input = args[0]
	}
	if input == "" {
		input = "-"
	}

	key, err := o.keys.given()
	if err != nil {
		return err
	}
	if key == nil && o.verify {
		if key, err = o.keys.resolve(false); err != nil {
			return err
		}
	}

	in, err := cli.OpenInput(input)
	if err != nil {
		return err
	}
	defer in.Close()

	r, sealed := sniff(in)
	if !sealed {
		return vault.ErrNotSealed
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	defer w.Flush()

	if key == nil {
		// Header only: no key needed, nothing secret is revealed.
		h, _, err := vault.ReadHeader(r)
		if err != nil {
			return err
		}
		printHeader(w, h)
		fmt.Fprintln(w, "integrity:\tnot checked (use --verify)")
		return nil
	}

	vr, err := vault.NewReader(r, vault.NewPassphrase(key))
	if err != nil {
		return err
	}
	printHeader(w, vr.Header)

	if md := vr.Metadata; md != nil {
		fmt.Fprintf(w, "name:\t%s\n", md.Name)
		fmt.Fprintf(w, "size:\t%d bytes\n", md.Size)
		if md.Mode != 0 {
			fmt.Fprintf(w, "mode:\t%s\n", md.Mode)
		}
		if !md.ModTime.IsZero() {
			fmt.Fprintf(w, "modified:\t%s\n", md.ModTime.Local().Format("2006-01-02 15:04:05 MST"))
		}
	}

	n, err := io.Copy(io.Discard, vr)
	if err != nil {
		w.Flush()
		return err
	}
	if vr.Metadata != nil && n != vr.Metadata.Size {
		w.Flush()
		return fmt.Errorf("payload is %d bytes, metadata says %d", n, vr.Metadata.Size)
	}
	fmt.Fprintf(w, "integrity:\tOK (%d bytes authenticated)\n", n)
	return nil
}

func printHeader(w io.Writer, h *vault.Header) {
	fmt.Fprintf(w, "format:\t%s v%d\n", vault.Magic, h.Version)
	fmt.Fprintln(w, "cipher:\tAES-256-GCM, STREAM chunks")
	fmt.Fprintf(w, "chunk size:\t%d KiB\n", h.ChunkSize()>>10)
	fmt.Fprintf(w, "kdf:\tPBKDF2-HMAC-SHA256, %d iterations\n", h.Iterations)
	fmt.Fprintf(w, "kdf salt:\t%x\n", h.KDFSalt)
	fmt.Fprintf(w, "compressed:\t%v\n", h.Compressed())
	fmt.Fprintf(w, "metadata:\t%v\n", h.HasMetadata())
}
