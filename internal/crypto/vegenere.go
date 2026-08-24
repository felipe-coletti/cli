package crypto

import "errors"

func vigenereEncryptBytes(data, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("key cannot be empty")
	}
	result := make([]byte, len(data))
	keyIndex := 0
	for i, b := range data {
		if b >= 'A' && b <= 'Z' {
			shift := int((key[keyIndex%len(key)] - 'A') % 26)
			result[i] = 'A' + byte((int(b-'A')+shift)%26)
			keyIndex++
		} else if b >= 'a' && b <= 'z' {
			k := key[keyIndex%len(key)]
			if k >= 'A' && k <= 'Z' {
				k = k + 32
			}
			shift := int((k - 'a') % 26)
			result[i] = 'a' + byte((int(b-'a')+shift)%26)
			keyIndex++
		} else {
			result[i] = b
		}
	}
	return result, nil
}

func vigenereDecryptBytes(data, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("key cannot be empty")
	}
	result := make([]byte, len(data))
	keyIndex := 0
	for i, b := range data {
		if b >= 'A' && b <= 'Z' {
			shift := int((key[keyIndex%len(key)] - 'A') % 26)
			result[i] = 'A' + byte((int(b-'A')-shift+26)%26)
			keyIndex++
		} else if b >= 'a' && b <= 'z' {
			k := key[keyIndex%len(key)]
			if k >= 'A' && k <= 'Z' {
				k = k + 32
			}
			shift := int((k - 'a') % 26)
			result[i] = 'a' + byte((int(b-'a')-shift+26)%26)
			keyIndex++
		} else {
			result[i] = b
		}
	}
	return result, nil
}
