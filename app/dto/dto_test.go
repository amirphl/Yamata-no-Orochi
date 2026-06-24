package dto

import (
	"strings"
	"testing"
)

func TestMaskPhoneNumber(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		check func(t *testing.T, got string)
	}{
		{
			name:  "typical Iranian mobile",
			input: "+989123456789",
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.HasPrefix(got, "+989123") {
					t.Errorf("expected prefix +989123, got %q", got)
				}
				if !strings.Contains(got, "*****") {
					t.Errorf("expected asterisks in masked number, got %q", got)
				}
				if strings.Contains(got, "456789") {
					t.Errorf("tail digits should be masked, got %q", got)
				}
			},
		},
		{
			name:  "exactly 10 chars gets asterisk suffix",
			input: "0123456789",
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.HasPrefix(got, "0123456") {
					t.Errorf("expected first 7 chars preserved, got %q", got)
				}
				if !strings.HasSuffix(got, "*****") {
					t.Errorf("expected '*****' suffix, got %q", got)
				}
			},
		},
		{
			name:  "short number under 8 chars returned as-is",
			input: "1234567",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "1234567" {
					t.Errorf("short number should be unchanged, got %q", got)
				}
			},
		},
		{
			name:  "exactly 8 chars masked with middle stars",
			input: "12345678",
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.Contains(got, "*****") {
					t.Errorf("expected asterisks in 8-char mask, got %q", got)
				}
			},
		},
		{
			name:  "empty string returned as-is",
			input: "",
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "" {
					t.Errorf("empty input should return empty, got %q", got)
				}
			},
		},
		{
			name:  "masked result always contains asterisks when input >= 8",
			input: "12345678901",
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.Contains(got, "*") {
					t.Errorf("expected masking stars, got %q", got)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := MaskPhoneNumber(tc.input)
			tc.check(t, got)
		})
	}
}
