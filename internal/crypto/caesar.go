package crypto

import (
	"errors"
	"fmt"
)

func caesarEncryptBytes(data []byte, shift int) []byte {
	result := make([]byte, len(data))
	for i, b := range data {
		if b >= 'A' && b <= 'Z' {
			result[i] = 'A' + byte((int(b-'A')+shift)%26)
		} else if b >= 'a' && b <= 'z' {
			result[i] = 'a' + byte((int(b-'a')+shift)%26)
		} else {
			result[i] = b
		}
	}
	return result
}

func caesarDecryptBytes(data []byte, shift int) []byte {
	return caesarEncryptBytes(data, 26-shift)
}

func parseShift(key string) (int, error) {
	var shift int

	_, err := fmt.Sscanf(key, "%d", &shift)
	if err != nil {
		return 0, errors.New("key must be an integer for Caesar cipher")
	}

	return shift, nil
}
