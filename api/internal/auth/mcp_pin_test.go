package auth

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestMCPPINLifetimeIsThreeMinutes(t *testing.T) {
	if mcpPINTTL != 3*time.Minute {
		t.Fatalf("expected MCP PIN lifetime to be three minutes, got %s", mcpPINTTL)
	}
}

func TestValidateMCPPINRequiresExactlyFourASCIIDigits(t *testing.T) {
	tests := map[string]bool{
		"0000":  true,
		"1234":  true,
		"123":   false,
		"12345": false,
		"12a4":  false,
		"１２３４":  false,
		" 1234": false,
	}
	for pin, valid := range tests {
		t.Run(pin, func(t *testing.T) {
			err := ValidateMCPPIN(pin)
			if (err == nil) != valid {
				t.Fatalf("ValidateMCPPIN(%q) error = %v, valid = %v", pin, err, valid)
			}
		})
	}
}

func TestHashMCPPINUsesBcryptAndDoesNotExposePlaintext(t *testing.T) {
	hash, err := HashMCPPIN("4821", bcrypt.MinCost)
	if err != nil {
		t.Fatalf("HashMCPPIN returned error: %v", err)
	}
	if hash == "4821" || hash == "" {
		t.Fatalf("expected a non-empty bcrypt hash, got %q", hash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("4821")); err != nil {
		t.Fatalf("expected the original PIN to verify: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("4822")); err == nil {
		t.Fatal("expected a different PIN to be rejected")
	}
}
