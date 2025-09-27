package password

import (
	"crypto/rand"
	"math/big"
)

func Generate(length int, uppercase, lowercase, numbers, symbols bool) string {
	const (
		upperSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lowerSet = "abcdefghijklmnopqrstuvwxyz"
		numSet   = "0123456789"
		symSet   = "!@#$%^&*()-_=+"
	)

	charset := ""

	if uppercase {
		charset += upperSet
	}
	if lowercase {
		charset += lowerSet
	}
	if numbers {
		charset += numSet
	}
	if symbols {
		charset += symSet
	}

	if len(charset) == 0 {
		charset += "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()-_=+"
	}

	result := make([]byte, length)

	for i := range length {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		
		result[i] = charset[num.Int64()]
	}

	return string(result)
}
