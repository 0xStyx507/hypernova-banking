package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestValidateRegistrationNormalizesEmail(t *testing.T) {
	validated, err := ValidateRegistration(RegisterInput{
		Email:    "  Person@Example.com ",
		Password: "Safe-password1!",
		FullName: "Person Example",
	})
	if err != nil {
		t.Fatalf("validate registration: %v", err)
	}
	if validated.Email != "person@example.com" {
		t.Fatalf("expected normalized email, got %q", validated.Email)
	}
}

func TestValidateRegistrationPasswordBoundaries(t *testing.T) {
	for _, test := range []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "seven bytes", password: strings.Repeat("a", 7), valid: false},
		{name: "eight bytes", password: strings.Repeat("a", 8), valid: true},
		{name: "seventy two bytes", password: strings.Repeat("a", 72), valid: true},
		{name: "seventy three bytes", password: strings.Repeat("a", 73), valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateRegistration(RegisterInput{Email: "person@example.com", Password: test.password, FullName: "Person Example"})
			if (err == nil) != test.valid {
				t.Fatalf("expected valid=%t, got error=%v", test.valid, err)
			}
		})
	}
}

func TestHashPasswordDoesNotReturnPlaintext(t *testing.T) {
	password := "Safe-password1!"
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
		Password: "Safe-password1!",
		FullName: "Person Example",
	}); err == nil {
		t.Fatal("expected display-name email to be rejected")
	}
}

func TestValidateRegistrationAcceptsAccentsAndRejectsSymbols(t *testing.T) {
	for _, name := range []string{"María José", "Ñusta Pérez", "José Alvarez"} {
		if _, err := ValidateRegistration(RegisterInput{
			Email:    "person@example.com",
			Password: "Safe-password1!",
			FullName: name,
		}); err != nil {
			t.Fatalf("expected international name %q to pass: %v", name, err)
		}
	}
	for _, name := range []string{"Ana_123", "Ana\nPerez", "Ana/Perez"} {
		if _, err := ValidateRegistration(RegisterInput{
			Email:    "person@example.com",
			Password: "Safe-password1!",
			FullName: name,
		}); err == nil {
			t.Fatalf("expected invalid name %q to be rejected", name)
		}
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
