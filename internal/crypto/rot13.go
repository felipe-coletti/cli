package crypto

func rot13Bytes(data []byte) []byte {
	result := make([]byte, len(data))
	for i, b := range data {
		if b >= 'A' && b <= 'Z' {
			result[i] = 'A' + (b-'A'+13)%26
		} else if b >= 'a' && b <= 'z' {
			result[i] = 'a' + (b-'a'+13)%26
		} else {
			result[i] = b
		}
	}
	return result
}
