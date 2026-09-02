// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"crypto/sha3"
	"math/big"
	"strings"
)

const base62 = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// luidLen is the fixed LUID length: 5 hash bytes (40 bits) always encode
// to at most 7 base62 digits.
const luidLen = 7

// computeLUID produces a stable 7-char identifier from the page href.
// Mirrors Python: shake256("luid\0" + href).digest(5), base62 encoded.
func computeLUID(href string) string {
	h := sha3.NewSHAKE256()
	h.Write([]byte("luid\x00" + href))
	buf := make([]byte, 5)
	h.Read(buf)
	s := toBase62(buf)
	// Left-pad with '0' so values below 62^6 still fill 7 characters.
	return strings.Repeat("0", luidLen-len(s)) + s
}

// toBase62 converts big-endian bytes to a base62 string.
func toBase62(b []byte) string {
	n := new(big.Int).SetBytes(b)
	if n.Sign() == 0 {
		return "0"
	}
	base := big.NewInt(62)
	digits := make([]byte, 0, 8)
	for n.Sign() > 0 {
		mod := new(big.Int)
		n.QuoRem(n, base, mod)
		digits = append(digits, base62[mod.Int64()])
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
