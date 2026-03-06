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
	case "vigenere":
		return vigenereEncrypt(message, key)
	case "xor":
		return xorEncrypt(message, key)
	case "rot13":
		return rot13(message), nil
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
	case "vigenere":
		return vigenereDecrypt(message, key)
	case "xor":
		return xorEncrypt(message, key)
	case "rot13":
		return rot13(message), nil
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

func vigenereEncrypt(text, key string) (string, error) {
	if key == "" {
		return "", errors.New("Vigenère key cannot be empty")
	}

	var result strings.Builder

	key = strings.ToUpper(key)
	keyLen := len(key)
	keyIndex := 0

	for _, r := range text {
		if unicode.IsLetter(r) {
			base := 'A'

			if unicode.IsLower(r) {
				base = 'a'
			}

			shift := int(key[keyIndex%keyLen]) - 'A'
			shifted := (int(r)-int(base)+shift)%26 + int(base)

			result.WriteRune(rune(shifted))

			keyIndex++
		} else {
			result.WriteRune(r)
		}
	}

	return result.String(), nil
}

func vigenereDecrypt(text, key string) (string, error) {
	if key == "" {
		return "", errors.New("Vigenère key cannot be empty")
	}

	var result strings.Builder

	key = strings.ToUpper(key)
	keyLen := len(key)
	keyIndex := 0

	for _, r := range text {
		if unicode.IsLetter(r) {
			base := 'A'

			if unicode.IsLower(r) {
				base = 'a'
			}

			shift := int(key[keyIndex%keyLen]) - 'A'
			shifted := (int(r)-int(base)-shift+26)%26 + int(base)

			result.WriteRune(rune(shifted))

			keyIndex++
		} else {
			result.WriteRune(r)
		}
	}

	return result.String(), nil
}

func xorEncrypt(text, key string) (string, error) {
	if key == "" {
		return "", errors.New("XOR key cannot be empty")
	}

	var result []rune
	keyRunes := []rune(key)
	textRunes := []rune(text)

	for i, r := range textRunes {
		k := keyRunes[i%len(keyRunes)]
		result = append(result, r^k)
	}

	return string(result), nil
}

func rot13(text string) string {
	var result strings.Builder

	for _, r := range text {
		if r >= 'A' && r <= 'Z' {
			result.WriteRune('A' + (r-'A'+13)%26)
		} else if r >= 'a' && r <= 'z' {
			result.WriteRune('a' + (r-'a'+13)%26)
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}
