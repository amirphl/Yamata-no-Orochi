package businessflow

import (
	"strings"
	"testing"
)

func TestHashOTPCode(t *testing.T) {
	t.Parallel()

	hash1 := hashOTPCode("123456")
	hash2 := hashOTPCode("123456")
	if hash1 != hash2 {
		t.Fatal("same input must produce same hash")
	}

	hash3 := hashOTPCode("654321")
	if hash1 == hash3 {
		t.Fatal("different inputs must produce different hashes")
	}

	if len(hash1) != 64 {
		t.Fatalf("expected 64-char hex digest, got %d", len(hash1))
	}
}

func TestVerifyOTPCodeHash(t *testing.T) {
	t.Parallel()

	code := "123456"
	hash := hashOTPCode(code)

	if !verifyOTPCodeHash(code, hash) {
		t.Fatal("correct code should verify against its hash")
	}

	if verifyOTPCodeHash("000000", hash) {
		t.Fatal("wrong code should not verify")
	}

	if verifyOTPCodeHash("", hash) {
		t.Fatal("empty code should not verify against non-empty hash")
	}
}

func TestNormalizeEmailIdentifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input, want string
	}{
		{"User@Example.COM", "user@example.com"},
		{"  hello@example.com  ", "hello@example.com"},
		{"UPPERCASE@DOMAIN.ORG", "uppercase@domain.org"},
		{"already@lower.com", "already@lower.com"},
	}

	for _, tc := range cases {
		got := normalizeEmailIdentifier(tc.input)
		if got != tc.want {
			t.Errorf("normalizeEmailIdentifier(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeLoginIdentifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input, want string
	}{
		{"User@Example.COM", "user@example.com"},
		{"  user@example.com  ", "user@example.com"},
		{"+989123456789", "+989123456789"},
		{"  +989123456789  ", "+989123456789"},
		{"plaintext", "plaintext"},
	}

	for _, tc := range cases {
		got := normalizeLoginIdentifier(tc.input)
		if got != tc.want {
			t.Errorf("normalizeLoginIdentifier(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestIsSixDigitCode(t *testing.T) {
	t.Parallel()

	valid := []string{"000000", "123456", "999999", "010101"}
	for _, c := range valid {
		if !isSixDigitCode(c) {
			t.Errorf("isSixDigitCode(%q) should be true", c)
		}
	}

	invalid := []string{"12345", "1234567", "12345a", "abcdef", "", "12 456", " 23456"}
	for _, c := range invalid {
		if isSixDigitCode(c) {
			t.Errorf("isSixDigitCode(%q) should be false", c)
		}
	}
}

func TestMaskOTPTarget(t *testing.T) {
	t.Parallel()

	t.Run("phone number", func(t *testing.T) {
		masked := maskOTPTarget("+989123456789")
		if !strings.HasPrefix(masked, "+989123") {
			t.Errorf("unexpected phone mask: %q", masked)
		}
		if strings.Contains(masked, "456789") {
			t.Errorf("phone mask should hide tail digits, got: %q", masked)
		}
	})

	t.Run("email with long local part", func(t *testing.T) {
		masked := maskOTPTarget("user@example.com")
		if !strings.Contains(masked, "@example.com") {
			t.Errorf("email mask should preserve domain, got: %q", masked)
		}
		if !strings.HasPrefix(masked, "us") {
			t.Errorf("email mask should keep first 2 chars of local part, got: %q", masked)
		}
		if strings.Contains(masked, "er") {
			t.Errorf("email mask should hide rest of local part, got: %q", masked)
		}
	})

	t.Run("email with very short local part", func(t *testing.T) {
		masked := maskOTPTarget("a@b.com")
		if masked != "***" {
			t.Errorf("short local part should return ***, got: %q", masked)
		}
	})

	t.Run("uppercase email is normalised before masking", func(t *testing.T) {
		masked := maskOTPTarget("USER@EXAMPLE.COM")
		if !strings.Contains(masked, "@example.com") {
			t.Errorf("expected lowercase domain in mask, got: %q", masked)
		}
	})
}

func TestLoginFailureKey(t *testing.T) {
	t.Parallel()

	key1 := loginFailureKey("user@example.com", "192.168.1.1")
	key2 := loginFailureKey("user@example.com", "192.168.1.1")
	if key1 != key2 {
		t.Fatal("same inputs must produce same key")
	}

	key3 := loginFailureKey("other@example.com", "192.168.1.1")
	if key1 == key3 {
		t.Fatal("different identifier must produce different key")
	}

	key4 := loginFailureKey("user@example.com", "10.0.0.1")
	if key1 == key4 {
		t.Fatal("different IP must produce different key")
	}

	if !strings.HasPrefix(key1, "auth:login:fail:") {
		t.Fatalf("key should have expected prefix, got: %q", key1)
	}
}
