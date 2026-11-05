package vault

import (
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
)

const fileKeyInfo = "cli-vault/v1 payload"

// SealKey is a derived master key used to encrypt one or more files.
// Deriving it is the slow step (PBKDF2); reuse one SealKey for a whole batch.
type SealKey struct {
	master     []byte
	salt       [kdfSaltSize]byte
	iterations uint32
}

// NewSealKey derives a master key from passphrase with a fresh random salt.
func NewSealKey(passphrase []byte, iterations int) (*SealKey, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("empty passphrase")
	}
	if iterations <= 0 || iterations > MaxIterations {
		return nil, fmt.Errorf("iteration count %d out of range", iterations)
	}

	k := &SealKey{iterations: uint32(iterations)}
	if _, err := rand.Read(k.salt[:]); err != nil {
		return nil, err
	}

	master, err := pbkdf2.Key(sha256.New, string(passphrase), k.salt[:], iterations, 32)
	if err != nil {
		return nil, err
	}
	k.master = master
	return k, nil
}

// Passphrase opens sealed files. It caches derived master keys by
// (salt, iterations), so decrypting a batch produced by one run costs a
// single PBKDF2 derivation. It is safe for concurrent use.
type Passphrase struct {
	secret string

	mu    sync.Mutex
	cache map[cacheKey]*cacheEntry
}

type cacheKey struct {
	salt       [kdfSaltSize]byte
	iterations uint32
}

type cacheEntry struct {
	once sync.Once
	key  []byte
	err  error
}

// NewPassphrase returns a key source for decryption.
func NewPassphrase(secret []byte) *Passphrase {
	return &Passphrase{secret: string(secret), cache: map[cacheKey]*cacheEntry{}}
}

func (p *Passphrase) master(h *Header) ([]byte, error) {
	ck := cacheKey{salt: h.KDFSalt, iterations: h.Iterations}

	p.mu.Lock()
	e, ok := p.cache[ck]
	if !ok {
		e = &cacheEntry{}
		p.cache[ck] = e
	}
	p.mu.Unlock()

	e.once.Do(func() {
		e.key, e.err = pbkdf2.Key(sha256.New, p.secret, h.KDFSalt[:], int(h.Iterations), 32)
	})
	return e.key, e.err
}

func fileKey(master []byte, fileSalt []byte) ([]byte, error) {
	return hkdf.Key(sha256.New, master, fileSalt, fileKeyInfo, 32)
}
