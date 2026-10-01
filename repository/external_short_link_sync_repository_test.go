package repository

import "testing"

func TestExternalStoredLongURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "scheme-less", raw: "example.com/offer", want: "https://example.com/offer"},
		{name: "protocol-relative", raw: "//example.com/offer", want: "https://example.com/offer"},
		{name: "existing HTTPS is untouched", raw: "https://example.com/offer", want: "https://example.com/offer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := externalStoredLongURL(test.raw)
			if err != nil {
				t.Fatalf("externalStoredLongURL(%q): %v", test.raw, err)
			}
			if got != test.want {
				t.Fatalf("externalStoredLongURL(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestExternalStoredLongURLRejectsNonWebScheme(t *testing.T) {
	if _, err := externalStoredLongURL("mailto:person@example.com"); err == nil {
		t.Fatal("externalStoredLongURL() error = nil, want error")
	}
}
