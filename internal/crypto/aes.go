package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

func aesEncrypt(plaintext, key []byte) ([]byte, error) {
	key = normalizeAESKey(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

func aesDecrypt(ciphertext, key []byte) ([]byte, error) {
	key = normalizeAESKey(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("decryption failed (wrong key or corrupted data)")
	}

	return plaintext, nil
}

func normalizeAESKey(key []byte) []byte {
	if len(key) >= 32 {
		return key[:32]
	} else if len(key) >= 24 {
		return key[:24]
	} else if len(key) >= 16 {
		return key[:16]
	} else {
		newKey := make([]byte, 16)

		for i := range newKey {
			newKey[i] = key[i%len(key)]
		}

		return newKey
	}
}
