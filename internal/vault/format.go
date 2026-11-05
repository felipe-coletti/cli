// Package vault implements the CLI's encrypted file format (v1).
//
// A sealed file is a fixed 60-byte plaintext header followed by a stream of
// AES-256-GCM chunks:
//
//	offset size  field
//	0      4     magic        meta.Magic (e.g. "CLIV")
//	4      1     version      0x01
//	5      1     flags        bit 0: an encrypted metadata record precedes the payload
//	                          bit 1: the payload is gzip-compressed before encryption
//	6      1     kdf          0x01 = PBKDF2-HMAC-SHA256
//	7      1     chunk shift  log2(plaintext chunk size), 16 => 64 KiB
//	8      4     iterations   PBKDF2 iteration count (big endian)
//	12     16    kdf salt     PBKDF2 salt
//	28     32    file salt    HKDF salt, unique per file
//	60     ...   chunks       each: ciphertext(<= chunk size) || tag(16)
//
// Key schedule:
//
//	master   = PBKDF2-SHA256(passphrase, kdf salt, iterations, 32)
//	file key = HKDF-SHA256(master, file salt, "cli-vault/v1 payload", 32)
//
// The expensive PBKDF2 step depends only on (passphrase, kdf salt,
// iterations), so a batch of files encrypted in one run shares it and pays
// for it once; HKDF with a random 32-byte file salt still gives every file
// its own key.
//
// Because every file key is unique, chunk nonces can be deterministic: an
// 11-byte big-endian chunk counter followed by a 1-byte "last chunk" flag
// (the STREAM construction). This makes reordering, dropping, duplicating
// or truncating chunks detectable. The whole 60-byte header is passed as
// additional authenticated data to every chunk, so flipping any header bit
// also makes decryption fail.
//
// Metadata (original name, size, mode, mtime) is stored *inside* the
// encrypted stream as a length-prefixed JSON record, so it never leaks.
package vault

import (
	"bytes"
	"cli/internal/meta"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// Magic identifies a sealed file. It is defined in internal/meta.
	Magic = meta.Magic

	Version = 1

	HeaderSize = 60

	// FlagMetadata marks that the decrypted stream starts with a metadata record.
	FlagMetadata byte = 1 << 0

	// FlagGzip marks that the payload (after the metadata record) is gzip-compressed.
	FlagGzip byte = 1 << 1

	KDFPBKDF2SHA256 byte = 1

	DefaultChunkShift = 16 // 64 KiB

	// DefaultIterations follows the OWASP recommendation for PBKDF2-HMAC-SHA256.
	DefaultIterations = 600_000

	// MaxIterations bounds the work a hostile file can force on the reader.
	MaxIterations = 50_000_000

	minChunkShift = 10 // 1 KiB
	maxChunkShift = 24 // 16 MiB

	kdfSaltSize  = 16
	fileSaltSize = 32
	tagSize      = 16
)

var (
	ErrNotSealed   = errors.New("not a sealed file (bad magic)")
	ErrVersion     = errors.New("unsupported file format version")
	ErrAuth        = errors.New("decryption failed: wrong key or corrupted/tampered data")
	ErrTruncated   = errors.New("decryption failed: file is truncated")
	ErrBadHeader   = errors.New("invalid header")
	knownFlagsMask = FlagMetadata | FlagGzip
)

// Header is the plaintext header of a sealed file.
type Header struct {
	Version    byte
	Flags      byte
	KDF        byte
	ChunkShift byte
	Iterations uint32
	KDFSalt    [kdfSaltSize]byte
	FileSalt   [fileSaltSize]byte
}

// ChunkSize returns the plaintext size of a full chunk.
func (h *Header) ChunkSize() int { return 1 << h.ChunkShift }

// Compressed reports whether the payload is gzip-compressed.
func (h *Header) Compressed() bool { return h.Flags&FlagGzip != 0 }

// HasMetadata reports whether an encrypted metadata record is present.
func (h *Header) HasMetadata() bool { return h.Flags&FlagMetadata != 0 }

// MarshalBinary encodes the header in its on-disk form.
func (h *Header) MarshalBinary() ([]byte, error) {
	b := make([]byte, HeaderSize)
	copy(b[0:4], Magic)
	b[4] = h.Version
	b[5] = h.Flags
	b[6] = h.KDF
	b[7] = h.ChunkShift
	binary.BigEndian.PutUint32(b[8:12], h.Iterations)
	copy(b[12:28], h.KDFSalt[:])
	copy(b[28:60], h.FileSalt[:])
	return b, nil
}

// UnmarshalBinary decodes and validates a header.
func (h *Header) UnmarshalBinary(b []byte) error {
	if len(b) < HeaderSize {
		if IsSealed(b) {
			return fmt.Errorf("%w: %d bytes, need %d", ErrTruncated, len(b), HeaderSize)
		}
		return ErrNotSealed
	}
	if string(b[0:4]) != Magic {
		return ErrNotSealed
	}

	h.Version = b[4]
	h.Flags = b[5]
	h.KDF = b[6]
	h.ChunkShift = b[7]
	h.Iterations = binary.BigEndian.Uint32(b[8:12])
	copy(h.KDFSalt[:], b[12:28])
	copy(h.FileSalt[:], b[28:60])

	if h.Version != Version {
		return fmt.Errorf("%w: %d", ErrVersion, h.Version)
	}
	if h.Flags&^knownFlagsMask != 0 {
		return fmt.Errorf("%w: unknown flags 0x%02x", ErrBadHeader, h.Flags)
	}
	if h.KDF != KDFPBKDF2SHA256 {
		return fmt.Errorf("%w: unknown kdf %d", ErrBadHeader, h.KDF)
	}
	if h.ChunkShift < minChunkShift || h.ChunkShift > maxChunkShift {
		return fmt.Errorf("%w: chunk shift %d out of range", ErrBadHeader, h.ChunkShift)
	}
	if h.Iterations == 0 || h.Iterations > MaxIterations {
		return fmt.Errorf("%w: iteration count %d out of range", ErrBadHeader, h.Iterations)
	}
	return nil
}

// ReadHeader reads and parses the header from r, returning the raw bytes too
// (they are the AAD of every chunk).
func ReadHeader(r io.Reader) (*Header, []byte, error) {
	raw := make([]byte, HeaderSize)
	n, err := io.ReadFull(r, raw)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, nil, err
	}
	h := &Header{}
	if err := h.UnmarshalBinary(raw[:n]); err != nil {
		return nil, nil, err
	}
	return h, raw, nil
}

// IsSealed reports whether prefix starts with the format's magic bytes.
func IsSealed(prefix []byte) bool {
	return bytes.HasPrefix(prefix, []byte(Magic))
}
