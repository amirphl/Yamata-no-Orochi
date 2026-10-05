package businessflow

import "testing"

func TestSanitizeShortLinkDomainAcceptsJZBE(t *testing.T) {
	t.Parallel()
	domain := "jzbe.ir"
	got, err := sanitizeShortLinkDomain(&domain)
	if err != nil {
		t.Fatalf("sanitizeShortLinkDomain(jzbe.ir): %v", err)
	}
	if got == nil || *got != domain {
		t.Fatalf("sanitizeShortLinkDomain(jzbe.ir) = %v, want %q", got, domain)
	}
}

func TestCampaignTestShortLinkKeepsSMSBodySchemeLess(t *testing.T) {
	t.Parallel()
	messageLink := buildCampaignShortLink("jzbe.ir", "fjia6")
	if messageLink != "jzbe.ir/fjia6" {
		t.Fatalf("message short link = %q", messageLink)
	}
	if stored := canonicalShortLinkURL(messageLink); stored != "https://jzbe.ir/fjia6" {
		t.Fatalf("stored short link = %q", stored)
	}
}
