package checksumutil

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type algorithmSpec struct {
	hexLen int
	sumHex func([]byte) string
}

var algorithms = map[string]algorithmSpec{
	"md5": {
		hexLen: 32,
		sumHex: func(b []byte) string {
			sum := md5.Sum(b)
			return hex.EncodeToString(sum[:])
		},
	},
	"sha1": {
		hexLen: 40,
		sumHex: func(b []byte) string {
			sum := sha1.Sum(b)
			return hex.EncodeToString(sum[:])
		},
	},
	"sha256": {
		hexLen: 64,
		sumHex: func(b []byte) string {
			sum := sha256.Sum256(b)
			return hex.EncodeToString(sum[:])
		},
	},
}

// NormalizeAlgorithm returns the canonical algorithm key (lower-case, trimmed).
func NormalizeAlgorithm(algo string) string {
	return strings.ToLower(strings.TrimSpace(algo))
}

// IsSupportedAlgorithm reports whether the algorithm is recognized.
func IsSupportedAlgorithm(algo string) bool {
	_, ok := algorithms[NormalizeAlgorithm(algo)]
	return ok
}

// SupportedAlgorithms returns sorted canonical algorithm keys.
func SupportedAlgorithms() []string {
	out := make([]string, 0, len(algorithms))
	for k := range algorithms {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// HexLength returns expected lowercase/uppercase hex length for the algorithm.
func HexLength(algo string) (int, bool) {
	spec, ok := algorithms[NormalizeAlgorithm(algo)]
	if !ok {
		return 0, false
	}
	return spec.hexLen, true
}

// ComputeHex computes a checksum hex digest for the given algorithm.
func ComputeHex(algo string, b []byte) (string, error) {
	spec, ok := algorithms[NormalizeAlgorithm(algo)]
	if !ok {
		return "", fmt.Errorf("unsupported checksum algorithm %q", algo)
	}
	return spec.sumHex(b), nil
}

// SupportedAlgorithmsDisplay returns a human-readable "a|b|c" list.
func SupportedAlgorithmsDisplay() string {
	return strings.Join(SupportedAlgorithms(), "|")
}
