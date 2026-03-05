package crypto

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

func Encrypt(cipher, key, message string) (string, error) {
	switch cipher {
	case "cesar", "caesar":
		shift, err := parseShift(key)
		if err != nil {
			return "", err
		}
		return caesarEncrypt(message, shift), nil
	case "aes":
		return "", errors.New("AES not implemented yet")
	case "xor":
		return "", errors.New("XOR not implemented yet")
	default:
		return "", errors.New("unsupported cipher: " + cipher)
	}
}

func Decrypt(cipher, key, message string) (string, error) {
	switch cipher {
	case "cesar", "caesar":
		shift, err := parseShift(key)
		if err != nil {
			return "", err
		}
		return caesarDecrypt(message, shift), nil
	case "aes":
		return "", errors.New("AES not implemented yet")
	case "xor":
		return "", errors.New("XOR not implemented yet")
	default:
		return "", errors.New("unsupported cipher: " + cipher)
	}
}

func parseShift(key string) (int, error) {
	var shift int

	_, err := fmt.Sscanf(key, "%d", &shift)
	if err != nil {
		return 0, errors.New("key must be an integer for Caesar cipher")
	}

	return shift % 26, nil
}

func caesarEncrypt(text string, shift int) string {
	var result strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) {
			base := 'A'

			if unicode.IsLower(r) {
				base = 'a'
			}

			shifted := (int(r)-int(base)+shift)%26 + int(base)
			result.WriteRune(rune(shifted))
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

func caesarDecrypt(text string, shift int) string {
	return caesarEncrypt(text, 26-shift)
}
