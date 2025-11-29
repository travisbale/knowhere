package argon2

import (
	"errors"
	"strings"
	"testing"
)

func createTestHasher() *Hasher {
	return NewHasher(&Config{
		Memory:      64 * 1024, // 64 MB
		Iterations:  1,
		SaltLength:  16,
		KeyLength:   32,
		Parallelism: 4,
	})
}

// Hash Tests

func TestHash_Success(t *testing.T) {
	hasher := createTestHasher()

	input := "mySecurePassword123!"
	h, err := hasher.Hash(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if h == "" {
		t.Error("expected non-empty hash")
	}

	// Verify hash format: $argon2id$v=19$m=X,t=Y,p=Z$salt$hash
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Errorf("expected hash to start with $argon2id$, got %s", h)
	}

	parts := strings.Split(h, "$")
	if len(parts) != 6 {
		t.Errorf("expected 6 parts in hash, got %d", len(parts))
	}
}

func TestHash_DifferentSalts(t *testing.T) {
	hasher := createTestHasher()

	input := "sameInput"

	hash1, err := hasher.Hash(input)
	if err != nil {
		t.Fatalf("failed to hash (1): %v", err)
	}

	hash2, err := hasher.Hash(input)
	if err != nil {
		t.Fatalf("failed to hash (2): %v", err)
	}

	// Same input should produce different hashes due to different salts
	if hash1 == hash2 {
		t.Error("expected different hashes for same input (different salts)")
	}
}

func TestHash_EmptyInput(t *testing.T) {
	hasher := createTestHasher()

	h, err := hasher.Hash("")
	if err != nil {
		t.Fatalf("expected no error for empty input, got %v", err)
	}

	if h == "" {
		t.Error("expected non-empty hash even for empty input")
	}
}

func TestHash_LongInput(t *testing.T) {
	hasher := createTestHasher()

	// Test with a very long input
	longInput := strings.Repeat("a", 1000)
	h, err := hasher.Hash(longInput)
	if err != nil {
		t.Fatalf("expected no error for long input, got %v", err)
	}

	if h == "" {
		t.Error("expected non-empty hash")
	}
}

func TestHash_SpecialCharacters(t *testing.T) {
	hasher := createTestHasher()

	inputs := []string{
		"pass!@#$%^&*()",
		"pāsswørd",
		"密码",
		"🔐secure🔑",
	}

	for _, input := range inputs {
		h, err := hasher.Hash(input)
		if err != nil {
			t.Errorf("input %q: unexpected error: %v", input, err)
		}
		if h == "" {
			t.Errorf("input %q: expected non-empty hash", input)
		}
	}
}

// Verify Tests

func TestVerify_Success(t *testing.T) {
	hasher := createTestHasher()

	input := "correctInput"
	h, err := hasher.Hash(input)
	if err != nil {
		t.Fatalf("failed to hash: %v", err)
	}

	err = hasher.Verify(input, h)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestVerify_WrongInput(t *testing.T) {
	hasher := createTestHasher()

	input := "correctInput"
	h, err := hasher.Hash(input)
	if err != nil {
		t.Fatalf("failed to hash: %v", err)
	}

	err = hasher.Verify("wrongInput", h)
	if !errors.Is(err, ErrMismatchedHash) {
		t.Errorf("expected ErrMismatchedHash, got %v", err)
	}
}

func TestVerify_EmptyInput(t *testing.T) {
	hasher := createTestHasher()

	// Hash empty input
	h, err := hasher.Hash("")
	if err != nil {
		t.Fatalf("failed to hash empty input: %v", err)
	}

	// Verify with empty input
	err = hasher.Verify("", h)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Verify with non-empty input should fail
	err = hasher.Verify("notEmpty", h)
	if !errors.Is(err, ErrMismatchedHash) {
		t.Errorf("expected ErrMismatchedHash, got %v", err)
	}
}

func TestVerify_InvalidHashFormat(t *testing.T) {
	hasher := createTestHasher()

	invalidHashes := []string{
		"",
		"notahash",
		"$invalid$format",
		"$argon2id$v=19$m=65536",                // Too few parts
		"$wrong$v=19$m=65536,t=1,p=4$salt$hash", // Wrong algorithm
		"$argon2id$v=99$m=65536,t=1,p=4$salt$hash", // Wrong version
		"$argon2id$v=19$invalid$salt$hash",         // Invalid parameters
		"$argon2id$v=19$m=65536,t=1,p=4$!!!$hash",  // Invalid base64 salt
		"$argon2id$v=19$m=65536,t=1,p=4$salt$!!!",  // Invalid base64 hash
	}

	for _, h := range invalidHashes {
		err := hasher.Verify("input", h)
		if err == nil {
			t.Errorf("expected error for invalid hash: %s", h)
		}
	}
}

// Round-trip Tests

func TestRoundTrip_MultipleInputs(t *testing.T) {
	hasher := createTestHasher()

	inputs := []string{
		"simple",
		"Complex123!",
		"",
		"very long input with many words and characters 1234567890",
		"pāsswørd",
		"密码",
	}

	for _, input := range inputs {
		h, err := hasher.Hash(input)
		if err != nil {
			t.Errorf("input %q: failed to hash: %v", input, err)
			continue
		}

		// Verify correct input
		err = hasher.Verify(input, h)
		if err != nil {
			t.Errorf("input %q: verification failed: %v", input, err)
		}

		// Verify wrong input fails
		err = hasher.Verify(input+"wrong", h)
		if !errors.Is(err, ErrMismatchedHash) {
			t.Errorf("input %q: expected ErrMismatchedHash, got %v", input, err)
		}
	}
}

func TestRoundTrip_CaseSensitivity(t *testing.T) {
	hasher := createTestHasher()

	input := "Input123"
	h, err := hasher.Hash(input)
	if err != nil {
		t.Fatalf("failed to hash: %v", err)
	}

	// Correct case should work
	err = hasher.Verify("Input123", h)
	if err != nil {
		t.Errorf("expected verification to succeed with correct case, got %v", err)
	}

	// Wrong case should fail
	err = hasher.Verify("input123", h)
	if !errors.Is(err, ErrMismatchedHash) {
		t.Error("expected verification to fail with different case")
	}

	err = hasher.Verify("INPUT123", h)
	if !errors.Is(err, ErrMismatchedHash) {
		t.Error("expected verification to fail with different case")
	}
}

func TestRoundTrip_WhitespaceMatters(t *testing.T) {
	hasher := createTestHasher()

	input := "input"
	h, err := hasher.Hash(input)
	if err != nil {
		t.Fatalf("failed to hash: %v", err)
	}

	// Trailing space should fail
	err = hasher.Verify("input ", h)
	if !errors.Is(err, ErrMismatchedHash) {
		t.Error("expected verification to fail with trailing space")
	}

	// Leading space should fail
	err = hasher.Verify(" input", h)
	if !errors.Is(err, ErrMismatchedHash) {
		t.Error("expected verification to fail with leading space")
	}
}

// Configuration Tests

func TestHasher_DifferentConfigurations(t *testing.T) {
	configs := []*Config{
		{Memory: 16 * 1024, Iterations: 1, SaltLength: 16, KeyLength: 32, Parallelism: 2},
		{Memory: 64 * 1024, Iterations: 1, SaltLength: 16, KeyLength: 32, Parallelism: 4},
		{Memory: 256 * 1024, Iterations: 2, SaltLength: 32, KeyLength: 64, Parallelism: 8},
	}

	input := "testInput"

	for i, config := range configs {
		hasher := NewHasher(config)

		h, err := hasher.Hash(input)
		if err != nil {
			t.Errorf("config %d: failed to hash: %v", i, err)
			continue
		}

		// Hash should contain the configuration parameters
		if !strings.Contains(h, "$m=") {
			t.Errorf("config %d: hash missing memory parameter", i)
		}

		// Verify works with same hasher
		err = hasher.Verify(input, h)
		if err != nil {
			t.Errorf("config %d: verification failed: %v", i, err)
		}
	}
}

func TestHasher_CrossConfigurationVerification(t *testing.T) {
	// Hash with one configuration
	hasher1 := NewHasher(&Config{
		Memory:      64 * 1024,
		Iterations:  1,
		SaltLength:  16,
		KeyLength:   32,
		Parallelism: 4,
	})

	input := "testInput"
	h, err := hasher1.Hash(input)
	if err != nil {
		t.Fatalf("failed to hash: %v", err)
	}

	// Verify with different configuration hasher
	// Should still work because parameters are encoded in the hash
	hasher2 := NewHasher(&Config{
		Memory:      128 * 1024, // Different config
		Iterations:  2,
		SaltLength:  32,
		KeyLength:   64,
		Parallelism: 8,
	})

	err = hasher2.Verify(input, h)
	if err != nil {
		t.Errorf("verification should work with different hasher config (params in hash), got %v", err)
	}
}

// Security Tests

func TestHasher_ConstantTimeComparison(t *testing.T) {
	// This test can't directly verify constant-time behavior,
	// but we can ensure the comparison works correctly
	hasher := createTestHasher()

	input := "input"
	h, err := hasher.Hash(input)
	if err != nil {
		t.Fatalf("failed to hash: %v", err)
	}

	// Verify that similar but wrong inputs still fail
	similarInputs := []string{
		"inpu",   // One char short
		"input1", // One char extra
		"Input",  // Different case
		"inpuT",  // Last char different
	}

	for _, similar := range similarInputs {
		err := hasher.Verify(similar, h)
		if !errors.Is(err, ErrMismatchedHash) {
			t.Errorf("expected ErrMismatchedHash for similar input %q, got %v", similar, err)
		}
	}
}
