package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestValidateRegistrationNormalizesEmail(t *testing.T) {
	validated, err := ValidateRegistration(RegisterInput{
		Email:    "  Person@Example.com ",
		Password: "safe-password",
		FullName: "Person Example",
	})
	if err != nil {
		t.Fatalf("validate registration: %v", err)
	}
	if validated.Email != "person@example.com" {
		t.Fatalf("expected normalized email, got %q", validated.Email)
	}
}

func TestHashPasswordDoesNotReturnPlaintext(t *testing.T) {
	password := "safe-password"
	hash, err := HashPassword(password, bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == password {
		t.Fatal("password was returned in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatalf("compare password hash: %v", err)
	}
}

func TestValidateRegistrationRejectsDisplayEmail(t *testing.T) {
	if _, err := ValidateRegistration(RegisterInput{
		Email:    "Person <person@example.com>",
		Password: "safe-password",
		FullName: "Person Example",
	}); err == nil {
		t.Fatal("expected display-name email to be rejected")
	}
}

func TestValidateLoginRejectsIncompleteRequest(t *testing.T) {
	if _, err := ValidateLogin("person@example.com", ""); err == nil {
		t.Fatal("expected empty password to be rejected")
	}
	if _, err := ValidateLogin("not-an-email", "password"); err == nil {
		t.Fatal("expected malformed email to be rejected")
	}
}
