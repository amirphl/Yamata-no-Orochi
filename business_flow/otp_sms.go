package businessflow

import (
	"fmt"
	"strings"
)

func normalizeOTPMobile(mobile string) (string, error) {
	recipient := strings.TrimSpace(mobile)
	if strings.HasPrefix(recipient, "+") {
		recipient = recipient[1:]
	}
	if len(recipient) != 12 || !strings.HasPrefix(recipient, "989") {
		return "", fmt.Errorf("invalid mobile number format: %s", mobile)
	}
	return recipient, nil
}
