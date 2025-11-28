package argon2

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// ErrMismatchedHash is returned when the input doesn't match the hash
var ErrMismatchedHash = errors.New("argon2: input does not match hash")

// DefaultConfig provides OWASP-recommended Argon2id parameters
var DefaultConfig = &Config{
	Memory:      64 * 1024, // 64 MB
	Iterations:  2,
	SaltLength:  16,
	KeyLength:   32,
	Parallelism: 4,
}

// Hash generates an Argon2id hash using default parameters
func Hash(input string) (string, error) {
	c := DefaultConfig
	return hash(c.Memory, c.Iterations, c.SaltLength, c.KeyLength, c.Parallelism, input)
}

// hash generates an Argon2id hash with custom parameters
func hash(memory, iterations, saltLength, keyLength uint32, parallelism uint8, input string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	h := argon2.IDKey([]byte(input), salt, iterations, memory, parallelism, keyLength)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(h)

	// Encodes parameters in hash string so verification can use same settings
	// Format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
	encodedHash := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memory, iterations, parallelism, b64Salt, b64Hash)

	return encodedHash, nil
}

// Verify compares an Argon2id hash with a plaintext input.
// Returns nil on success or ErrMismatchedHash if the input doesn't match.
func Verify(input, encodedHash string) error {
	// Extracts parameters from encoded hash to ensure verification uses same settings as hashing
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return fmt.Errorf("invalid hash format")
	}

	if parts[1] != "argon2id" {
		return fmt.Errorf("unsupported algorithm: %s", parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return fmt.Errorf("failed to parse version: %w", err)
	}
	if version != argon2.Version {
		return fmt.Errorf("unsupported version: %d", version)
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return fmt.Errorf("failed to parse parameters: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return fmt.Errorf("failed to decode salt: %w", err)
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return fmt.Errorf("failed to decode hash: %w", err)
	}

	comparisonHash := argon2.IDKey([]byte(input), salt, time, memory, threads, uint32(len(hash)))

	// Constant-time comparison prevents timing attacks that could reveal password similarity
	if subtle.ConstantTimeCompare(hash, comparisonHash) != 1 {
		return ErrMismatchedHash
	}
	return nil
}
