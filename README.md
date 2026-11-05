# CLI

A personal command-line toolkit: authenticated file encryption, archives,
password generation and system info, in a single static binary with zero
dependencies outside the Go standard library.

## Build

```bash
make build            # static (CGO_ENABLED=0), stripped, version injected from git
make test             # go vet + go test -race
make release          # cross-compile into dist/ (linux, darwin, windows)
```

Or without make:

```bash
go run . [command]
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o mycli .
```

## Commands

```
mycli crypto encrypt|decrypt|inspect   encrypt files, streams or whole directories
mycli metadata show|strip              find and remove file metadata
mycli archive pack|extract             tar archives
mycli password generate                random passwords
mycli system info|date|id              system information
mycli help <command> [subcommand]     (or: <command> --help)
```

Exit codes: 0 success, 1 error, 2 invalid usage.

`archive extract` never writes outside the output directory: `..` paths are
rejected, symlinks pointing outside are refused and files are written
through `os.Root`.

### Encryption

```bash
# Without --key/--key-file/$MYCLI_KEY the passphrase is asked on the terminal
# (twice when encrypting). -k works too but ends up in shell history.
mycli crypto encrypt -i notes.txt -o notes.txt.mycli

mycli crypto encrypt --key-file ~/.key -i notes.txt -o notes.txt.mycli
mycli crypto decrypt --key-file ~/.key -i notes.txt.mycli -o .     # restores "notes.txt"

# Whole directory, files processed in parallel (one KDF run for the batch).
mycli crypto encrypt --key-file ~/.key -i ./photos -o ./photos.sealed
mycli crypto decrypt --key-file ~/.key -i ./photos.sealed -o ./photos

# Compose with archive for a single-file backup (-z compresses before encrypting).
mycli archive pack -i ./docs | mycli crypto encrypt -z --key-file ~/.key -o docs.tar.mycli

# Header without a key; with a key, verify every chunk without writing plaintext.
mycli crypto inspect docs.tar.mycli
mycli crypto inspect --verify docs.tar.mycli
```

Sealed files get the CLI name as extension (`.mycli`) in directory mode.
The classic ciphers (`-c xor|rot13|caesar|vigenere`) are kept for fun and
learning; they provide no real security.

### Metadata

```bash
mycli metadata show ./photos                       # what would be removed / kept
mycli metadata strip -i photo.jpg -o photo-clean.jpg
mycli metadata strip -i ./photos -o ./photos-clean # whole tree, in parallel
mycli metadata strip -i ./photos --in-place        # replace originals (atomic)
```

| Format | Removed |
|---|---|
| JPEG | EXIF (GPS, camera, serial numbers, dates, thumbnail), XMP, IPTC, comments, vendor blocks, data after the image |
| PNG | text chunks, eXIf, tIME and other non-rendering ancillary chunks |
| WebP | EXIF and XMP chunks (VP8X flags updated) |
| HEIC, HEIF, AVIF | EXIF item (GPS, camera, dates) and XMP item, overwritten in place |
| MP4, MOV, M4A, 3GP | user data and QuickTime metadata (location, make, model, software, title, artist…), XMP, timed metadata tracks (iPhone `mebx`, GoPro GPS), creation/modification times |
| PDF | document info (author, title, subject, keywords, creator, producer, dates) in every revision, XMP, document ID, comment authors, EXIF/XMP of embedded JPEG photos (also under ASCII85/ASCIIHex/Flate encoding); also inside compressed object streams |
| DOCX, XLSX, PPTX | docProps (author, last modified by, company, manager, template, editing time, custom properties), EXIF of embedded photos, zip timestamps and comment |

Directories are processed recursively. Image, audio and video data is never
re-encoded. MP4/MOV/HEIC and PDF files keep their exact size and layout: metadata
boxes become zero-filled `free` boxes and metadata items are overwritten in
place, so even multi-gigabyte videos are cleaned without being read into
memory, and `--in-place` only rewrites those few bytes. Color profiles and the EXIF orientation are kept (`--all` drops the
orientation too). Comments and tracked changes inside Office documents keep
their author names; `show` warns about it. Encrypted PDFs are left untouched,
and `show` warns that cleaning a signed PDF invalidates its signature. Unsupported files are skipped
(`--copy-unsupported` copies them unchanged in copy mode).

## Sealed file format (v1)

```
offset size  field
0      4     magic        "CLIV" (meta.Magic in internal/meta/meta.go)
4      1     version      0x01
5      1     flags        bit 0: encrypted metadata record present
                          bit 1: payload gzip-compressed before encryption
6      1     kdf          0x01 = PBKDF2-HMAC-SHA256
7      1     chunk shift  log2(chunk size), 16 = 64 KiB
8      4     iterations   big endian, default 600 000 (OWASP)
12     16    kdf salt
28     32    file salt
60     ...   AES-256-GCM chunks: ciphertext || 16-byte tag
```

- `master = PBKDF2(passphrase, kdf salt)`, `file key = HKDF-SHA256(master, file salt)`:
  a batch pays for PBKDF2 once and every file still gets its own key.
- Chunk nonce = 11-byte counter || last-chunk flag (STREAM construction):
  reordered, duplicated, dropped or truncated chunks are rejected.
- The 60-byte header is authenticated as AAD of every chunk.
- Name, size, mode and mtime are stored **inside** the ciphertext, never in clear.
- Constant memory regardless of file size; output files are written to a
  temporary file, fsynced and renamed, so failures never leave partial output.
