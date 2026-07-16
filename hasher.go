package main

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/sha3"
)

// HashAlgo represents a supported hash algorithm.
type HashAlgo int

const (
	AlgoSha3_224 HashAlgo = iota
	AlgoSha3_256
	AlgoSha3_384
	AlgoSha3_512
	AlgoSHA_224
	AlgoSHA_256
	AlgoSHA_384
	AlgoSHA_512
	AlgoSHA_512_224
	AlgoSHA_512_256
)

// HashConfig holds the parsed hash algorithm configuration.
type HashConfig struct {
	Algo       HashAlgo
	Truncate   int    // 0 = full hash, >0 = first N hex chars
	AlgoName   string // canonical display name, e.g. "sha3-224"
	TruncStr   string // display suffix, e.g. ":16"
}

// DisplayName returns the human-readable name, e.g. "sha3-224:16".
func (c HashConfig) DisplayName() string {
	if c.Truncate > 0 {
		return c.AlgoName + ":" + c.TruncStr
	}
	return c.AlgoName
}

// FullHexLen returns the full hex length for the algorithm.
func (c HashConfig) FullHexLen() int {
	switch c.Algo {
	case AlgoSha3_224:
		return sha3.New224().Size() * 2
	case AlgoSha3_256:
		return sha3.New256().Size() * 2
	case AlgoSha3_384:
		return sha3.New384().Size() * 2
	case AlgoSha3_512:
		return sha3.New512().Size() * 2
	case AlgoSHA_224:
		return sha256.New224().Size() * 2
	case AlgoSHA_256:
		return sha256.New().Size() * 2
	case AlgoSHA_384:
		return sha512.New384().Size() * 2
	case AlgoSHA_512:
		return sha512.New().Size() * 2
	case AlgoSHA_512_224:
		return sha512.New512_224().Size() * 2
	case AlgoSHA_512_256:
		return sha512.New512_256().Size() * 2
	default:
		return 0
	}
}

// newHasher creates the hash.Hash for this config.
func (c HashConfig) newHasher() hash.Hash {
	switch c.Algo {
	case AlgoSha3_224:
		return sha3.New224()
	case AlgoSha3_256:
		return sha3.New256()
	case AlgoSha3_384:
		return sha3.New384()
	case AlgoSha3_512:
		return sha3.New512()
	case AlgoSHA_224:
		return sha256.New224()
	case AlgoSHA_256:
		return sha256.New()
	case AlgoSHA_384:
		return sha512.New384()
	case AlgoSHA_512:
		return sha512.New()
	case AlgoSHA_512_224:
		return sha512.New512_224()
	case AlgoSHA_512_256:
		return sha512.New512_256()
	default:
		panic("unknown hash algorithm")
	}
}

// HashFile computes the hash of a file and returns its uppercase hex string.
func (c HashConfig) HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := c.newHasher()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}

	fullHex := hex.EncodeToString(h.Sum(nil))
	if c.Truncate > 0 && c.Truncate < len(fullHex) {
		return strings.ToUpper(fullHex[:c.Truncate]), nil
	}
	return strings.ToUpper(fullHex), nil
}

// Sha3512HashFile computes the SHA3-512 hash of a file (used for collision verification).
func Sha3512HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha3.New512()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil))), nil
}

// ParseHashConfig parses a string like "sha3-224", "SHA-256", "sha3-224:16", "sha256:8" etc.
// Case-insensitive, dash-optional for SHA-2 names.
func ParseHashConfig(raw string) (HashConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return HashConfig{}, fmt.Errorf("empty algorithm name")
	}

	// Split on last ':' for truncation
	var algoPart, truncPart string
	if idx := strings.LastIndex(raw, ":"); idx > 0 {
		algoPart = raw[:idx]
		truncPart = raw[idx+1:]
	} else {
		algoPart = raw
	}

	// Normalise: lowercase, then map underscore variants
	lower := strings.ToLower(algoPart)

	// Support "SHA-512/224" style — normalise everything but keep slashes
	// For SHA-2: "sha256" -> "sha-256", "sha384" -> "sha-384", etc.
	lower = normaliseAlgoName(lower)

	var algo HashAlgo
	var canonical string

	switch lower {
	case "sha3-224":
		algo = AlgoSha3_224
		canonical = "sha3-224"
	case "sha3-256":
		algo = AlgoSha3_256
		canonical = "sha3-256"
	case "sha3-384":
		algo = AlgoSha3_384
		canonical = "sha3-384"
	case "sha3-512":
		algo = AlgoSha3_512
		canonical = "sha3-512"
	case "sha-224":
		algo = AlgoSHA_224
		canonical = "SHA-224"
	case "sha-256":
		algo = AlgoSHA_256
		canonical = "SHA-256"
	case "sha-384":
		algo = AlgoSHA_384
		canonical = "SHA-384"
	case "sha-512":
		algo = AlgoSHA_512
		canonical = "SHA-512"
	case "sha-512/224":
		algo = AlgoSHA_512_224
		canonical = "SHA-512/224"
	case "sha-512/256":
		algo = AlgoSHA_512_256
		canonical = "SHA-512/256"
	default:
		return HashConfig{}, fmt.Errorf("unsupported hash algorithm: %q\nsupported: sha3-224, sha3-256, sha3-384, sha3-512, SHA-224, SHA-256, SHA-384, SHA-512, SHA-512/224, SHA-512/256", algoPart)
	}

	cfg := HashConfig{Algo: algo, AlgoName: canonical}

	// Parse truncation
	if truncPart != "" {
		var n int
		if _, err := fmt.Sscanf(truncPart, "%d", &n); err != nil || n <= 0 {
			return HashConfig{}, fmt.Errorf("invalid truncation value: %q (must be positive integer)", truncPart)
		}
		fullLen := cfg.FullHexLen()
		if n >= fullLen {
			// Truncation >= full size is pointless, just use full
			cfg.Truncate = 0
		} else {
			cfg.Truncate = n
			cfg.TruncStr = truncPart
		}
	}

	return cfg, nil
}

// normaliseAlgoName lowercases and normalises variant spellings.
//   "sha256" → "sha-256", "sha224" → "sha-224"
//   "sha384" → "sha-384", "sha512" → "sha-512"
//   "sha3224" → "sha3-224", "sha3512" → "sha3-512"
//   "sha512/224" → "sha-512/224"
func normaliseAlgoName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")

	// Already has a dash — nothing to normalise
	if strings.Contains(s, "-") {
		return s
	}

	// SHA-2: "sha224" / "sha256" / "sha384" / "sha512" (6 chars each)
	// "sha" + 3 digit chars
	if len(s) == 6 && strings.HasPrefix(s, "sha") {
		digits := s[3:]
		if digits >= "224" && digits <= "512" {
			return "sha-" + digits
		}
	}

	// SHA-2 with variant: "sha512/224" → "sha-512/224"
	if strings.HasPrefix(s, "sha") && strings.Contains(s, "/") && !strings.Contains(s, "-") {
		rest := s[3:]
		if len(rest) > 3 {
			return "sha-" + rest
		}
	}

	// SHA-3: "sha3224" → "sha3-224" (7 chars), "sha3256" → "sha3-256" (7 chars)
	if len(s) == 7 && strings.HasPrefix(s, "sha3") {
		digits := s[4:]
		if digits >= "224" && digits <= "512" {
			return "sha3-" + digits
		}
	}

	return s
}
