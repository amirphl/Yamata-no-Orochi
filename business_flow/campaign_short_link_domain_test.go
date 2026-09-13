package businessflow

import "testing"

func TestSanitizeShortLinkDomainAcceptsJZBE(t *testing.T) {
	domain := "jzbe.ir"
	got, err := sanitizeShortLinkDomain(&domain)
	if err != nil {
		t.Fatalf("sanitizeShortLinkDomain(jzbe.ir): %v", err)
	}
	if got == nil || *got != domain {
		t.Fatalf("sanitizeShortLinkDomain(jzbe.ir) = %v, want %q", got, domain)
	}
}
