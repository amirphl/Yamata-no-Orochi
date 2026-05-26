package businessflow

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
)

const (
	authOTPMaxAttempts     = 5
	authOTPResendCooldown  = 30 * time.Second
	authLoginMaxFailures   = 5
	authLoginFailureWindow = 15 * time.Minute
)

type otpChallengeState struct {
	OTPHash    string    `json:"otp_hash"`
	Attempts   int       `json:"attempts"`
	CreatedAt  time.Time `json:"created_at"`
	LastSentAt time.Time `json:"last_sent_at"`
}

func hashOTPCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func verifyOTPCodeHash(code, expectedHash string) bool {
	actual := hashOTPCode(code)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func normalizeEmailIdentifier(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeLoginIdentifier(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if strings.Contains(identifier, "@") {
		return normalizeEmailIdentifier(identifier)
	}
	return identifier
}

func isSixDigitCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func maskOTPTarget(target string) string {
	if strings.Contains(target, "@") {
		target = normalizeEmailIdentifier(target)
		parts := strings.SplitN(target, "@", 2)
		if len(parts) != 2 || len(parts[0]) < 2 {
			return "***"
		}
		return fmt.Sprintf("%s***@%s", parts[0][:2], parts[1])
	}
	return dto.MaskPhoneNumber(target)
}

func loginFailureKey(identifier, ipAddress string) string {
	return fmt.Sprintf("auth:login:fail:%s:%s", hashOTPCode(identifier), hashOTPCode(ipAddress))
}
