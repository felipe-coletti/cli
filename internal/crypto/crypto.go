package crypto

import (
	"errors"
	"strings"
)

func Encrypt(cipherType, key string, data []byte) ([]byte, error) {
	switch strings.ToLower(cipherType) {
	case "aes":
		return aesEncrypt(data, []byte(key))

	case "xor":
		return xorEncryptBytes(data, []byte(key)), nil

	case "rot13":
		return rot13Bytes(data), nil

	case "cesar", "caesar":
		shift, err := parseShift(key)
		if err != nil {
			return nil, err
		}

		return caesarEncryptBytes(data, shift), nil

	case "vigenere":
		return vigenereEncryptBytes(data, []byte(key))

	default:
		return nil, errors.New("unsupported cipher: " + cipherType)
	}
}

func Decrypt(cipherType, key string, data []byte) ([]byte, error) {
	switch strings.ToLower(cipherType) {
	case "aes":
		return aesDecrypt(data, []byte(key))

	case "xor":
		return xorEncryptBytes(data, []byte(key)), nil

	case "rot13":
		return rot13Bytes(data), nil

	case "cesar", "caesar":
		shift, err := parseShift(key)
		if err != nil {
			return nil, err
		}

		return caesarDecryptBytes(data, shift), nil

	case "vigenere":
		return vigenereDecryptBytes(data, []byte(key))

	default:
		return nil, errors.New("unsupported cipher: " + cipherType)
	}
}
