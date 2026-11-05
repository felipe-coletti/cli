package vault

import (
	"bufio"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"
)

const maxMetadataSize = 64 << 10

// Metadata describes the original file. It is encrypted together with the payload.
type Metadata struct {
	Name    string      `json:"name,omitempty"`
	Size    int64       `json:"size"`
	Mode    fs.FileMode `json:"mode,omitempty"`
	ModTime time.Time   `json:"mtime,omitzero"`
}

// MetadataFromFileInfo builds Metadata for a regular file.
func MetadataFromFileInfo(fi fs.FileInfo) *Metadata {
	return &Metadata{
		Name:    fi.Name(),
		Size:    fi.Size(),
		Mode:    fi.Mode().Perm(),
		ModTime: fi.ModTime().UTC().Truncate(time.Second),
	}
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// nonce builds the STREAM nonce: 11-byte big-endian counter || last flag.
func nonce(dst *[12]byte, counter uint64, last bool) {
	*dst = [12]byte{}
	binary.BigEndian.PutUint64(dst[3:11], counter)
	if last {
		dst[11] = 1
	}
}

// Options tune how a stream is sealed.
type Options struct {
	// Compress gzips the payload before encrypting it. Useful for text,
	// logs and similar data; pointless for already-compressed files
	// (photos, video, zip). Note that compression makes the ciphertext
	// size depend on the content.
	Compress bool
}

// Writer encrypts a stream. Close must be called to write the final chunk.
type Writer struct {
	chunks *chunkWriter
	gz     *gzip.Writer
	closed bool
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("vault: write after close")
	}
	if w.gz != nil {
		return w.gz.Write(p)
	}
	return w.chunks.Write(p)
}

// Close flushes the compressor (if any) and seals the final chunk. It does
// not close the underlying writer.
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if w.gz != nil {
		if err := w.gz.Close(); err != nil {
			return err
		}
	}
	return w.chunks.Close()
}

// chunkWriter implements the STREAM chunking.
type chunkWriter struct {
	dst     io.Writer
	aead    cipher.AEAD
	aad     []byte
	buf     []byte
	out     []byte
	chunk   int
	counter uint64
	closed  bool
	err     error
}

// NewWriter writes a header to dst and returns a writer that encrypts
// everything written to it. If md is non-nil it is stored encrypted ahead of
// the payload.
func NewWriter(dst io.Writer, key *SealKey, md *Metadata, opts Options) (*Writer, error) {
	h := Header{
		Version:    Version,
		KDF:        KDFPBKDF2SHA256,
		ChunkShift: DefaultChunkShift,
		Iterations: key.iterations,
		KDFSalt:    key.salt,
	}
	if md != nil {
		h.Flags |= FlagMetadata
	}
	if opts.Compress {
		h.Flags |= FlagGzip
	}
	if _, err := rand.Read(h.FileSalt[:]); err != nil {
		return nil, err
	}

	raw, _ := h.MarshalBinary()

	fk, err := fileKey(key.master, h.FileSalt[:])
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(fk)
	if err != nil {
		return nil, err
	}

	if _, err := dst.Write(raw); err != nil {
		return nil, err
	}

	chunk := h.ChunkSize()
	w := &chunkWriter{
		dst:   dst,
		aead:  aead,
		aad:   raw,
		chunk: chunk,
		buf:   make([]byte, 0, chunk),
		out:   make([]byte, 0, chunk+tagSize),
	}

	if md != nil {
		js, err := json.Marshal(md)
		if err != nil {
			return nil, err
		}
		if len(js) > maxMetadataSize {
			return nil, errors.New("metadata too large")
		}
		var lenPrefix [4]byte
		binary.BigEndian.PutUint32(lenPrefix[:], uint32(len(js)))
		if _, err := w.Write(lenPrefix[:]); err != nil {
			return nil, err
		}
		if _, err := w.Write(js); err != nil {
			return nil, err
		}
	}

	out := &Writer{chunks: w}
	if opts.Compress {
		out.gz = gzip.NewWriter(w)
	}
	return out, nil
}

func (w *chunkWriter) flush(last bool) error {
	var n [12]byte
	nonce(&n, w.counter, last)
	w.out = w.aead.Seal(w.out[:0], n[:], w.buf, w.aad)
	w.counter++
	w.buf = w.buf[:0]
	_, err := w.dst.Write(w.out)
	return err
}

// Write encrypts p. A full chunk is only sealed once more data arrives, so
// that Close can always mark a non-empty chunk as the last one.
func (w *chunkWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("vault: write after close")
	}
	if w.err != nil {
		return 0, w.err
	}

	total := 0
	for len(p) > 0 {
		if len(w.buf) == w.chunk {
			if w.err = w.flush(false); w.err != nil {
				return total, w.err
			}
		}
		n := copy(w.buf[len(w.buf):w.chunk], p)
		w.buf = w.buf[:len(w.buf)+n]
		p = p[n:]
		total += n
	}
	return total, nil
}

// Close seals the final chunk. It does not close the underlying writer.
func (w *chunkWriter) Close() error {
	if w.closed {
		return w.err
	}
	w.closed = true
	if w.err != nil {
		return w.err
	}
	w.err = w.flush(true)
	return w.err
}

// Reader decrypts (and decompresses) a stream produced by Writer.
type Reader struct {
	Header   *Header
	Metadata *Metadata

	payload io.Reader
}

func (r *Reader) Read(p []byte) (int, error) { return r.payload.Read(p) }

// chunkReader implements the STREAM chunking.
type chunkReader struct {
	src     *bufio.Reader
	aead    cipher.AEAD
	aad     []byte
	enc     []byte
	plain   []byte
	chunk   int
	counter uint64
	done    bool
	err     error
}

// NewReader parses the header from src, derives the file key with keys and
// returns a reader of the plaintext payload. Every byte returned by Read has
// been authenticated; an error is returned if the stream is tampered with or
// truncated, but only once the bad chunk is reached, so callers that write
// output must discard it on error (see cli.AtomicFile).
func NewReader(src io.Reader, keys *Passphrase) (*Reader, error) {
	br := bufio.NewReaderSize(src, 1<<16)

	h, raw, err := ReadHeader(br)
	if err != nil {
		return nil, err
	}

	master, err := keys.master(h)
	if err != nil {
		return nil, err
	}
	fk, err := fileKey(master, h.FileSalt[:])
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(fk)
	if err != nil {
		return nil, err
	}

	chunk := h.ChunkSize()
	r := &chunkReader{
		src:   br,
		aead:  aead,
		aad:   raw,
		chunk: chunk,
		enc:   make([]byte, chunk+tagSize),
	}

	out := &Reader{Header: h, payload: r}

	if h.HasMetadata() {
		var lenPrefix [4]byte
		if _, err := io.ReadFull(r, lenPrefix[:]); err != nil {
			return nil, metaErr(err)
		}
		n := binary.BigEndian.Uint32(lenPrefix[:])
		if n > maxMetadataSize {
			return nil, fmt.Errorf("%w: metadata record too large", ErrBadHeader)
		}
		js := make([]byte, n)
		if _, err := io.ReadFull(r, js); err != nil {
			return nil, metaErr(err)
		}
		md := &Metadata{}
		if err := json.Unmarshal(js, md); err != nil {
			return nil, fmt.Errorf("%w: metadata: %v", ErrBadHeader, err)
		}
		out.Metadata = md
	}

	if h.Compressed() {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, metaErr(err)
		}
		out.payload = gz
	}

	return out, nil
}

func metaErr(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrTruncated
	}
	return err
}

func (r *chunkReader) Read(p []byte) (int, error) {
	for len(r.plain) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		if r.done {
			return 0, io.EOF
		}
		r.err = r.next()
	}
	n := copy(p, r.plain)
	r.plain = r.plain[n:]
	return n, nil
}

func (r *chunkReader) next() error {
	n, err := io.ReadFull(r.src, r.enc)
	last := false

	switch {
	case errors.Is(err, io.EOF):
		// Clean EOF at a chunk boundary without having seen the final chunk.
		return ErrTruncated
	case errors.Is(err, io.ErrUnexpectedEOF):
		last = true
	case err != nil:
		return err
	default:
		// Full-size chunk: it is the last one only if nothing follows.
		if _, perr := r.src.Peek(1); errors.Is(perr, io.EOF) {
			last = true
		} else if perr != nil {
			return perr
		}
	}

	if n < tagSize {
		return ErrTruncated
	}

	var nn [12]byte
	nonce(&nn, r.counter, last)

	plain, err := r.aead.Open(r.enc[:0], nn[:], r.enc[:n], r.aad)
	if err != nil {
		// A chunk sealed as "not last" but found at EOF lands here too.
		return ErrAuth
	}

	// The writer never produces an empty final chunk except for an empty stream.
	if last && len(plain) == 0 && r.counter > 0 {
		return ErrAuth
	}

	r.counter++
	r.plain = plain
	r.done = last
	return nil
}
