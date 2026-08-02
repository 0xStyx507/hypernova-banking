package auth

import (
	"strings"
	"testing"
	"time"
)

func TestVerifyTOTPAcceptsCurrentWindow(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0).UTC()
	code := testTOTPCode(secret, now)

	if !VerifyTOTP(secret, code, now) {
		t.Fatal("expected current TOTP code to be accepted")
	}
	if VerifyTOTP(secret, "000000", now) && code != "000000" {
		t.Fatal("expected an unrelated TOTP code to be rejected")
	}
}

func TestOTPAuthURIUsesStandardAuthenticatorProfile(t *testing.T) {
	uri := otpAuthURI("JBSWY3DPEHPK3PXP", "person@example.com")
	for _, value := range []string{"otpauth://totp/", "issuer=Hypernova+Banking", "algorithm=SHA1", "digits=6", "period=30"} {
		if !strings.Contains(uri, value) {
			t.Fatalf("expected provisioning URI to contain %q: %s", value, uri)
		}
	}
}

func TestMFAEncryptionKeySupportsConfiguredFormats(t *testing.T) {
	key := strings.Repeat("a", 64)
	decoded, err := MFAEncryptionKey(key, "")
	if err != nil || len(decoded) != 32 {
		t.Fatalf("expected hexadecimal key to decode: len=%d err=%v", len(decoded), err)
	}
}
