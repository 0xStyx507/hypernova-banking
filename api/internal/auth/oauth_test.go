package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchGoogleOAuthProfileRequiresVerifiedEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "provider-token"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub": "google-subject", "email": "person@example.com", "name": "Person Example", "email_verified": false,
		})
	}))
	defer server.Close()

	_, err := fetchOAuthProfile(context.Background(), OAuthProviderGoogle, OAuthProviderConfig{
		ClientID: "client-id", ClientSecret: "client-secret", RedirectURL: "http://localhost/callback",
		TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/profile",
	}, "authorization-code", server.Client())
	if err != ErrOAuthIdentity {
		t.Fatalf("expected unverified identity rejection, got %v", err)
	}
}

func TestFetchGitHubOAuthProfileUsesProviderSubject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "provider-token"})
			return
		}
		if r.URL.Path == "/emails" {
			_ = json.NewEncoder(w).Encode([]map[string]any{{"email": "person@example.com", "primary": true, "verified": true}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 12345, "login": "person", "name": "Person Example"})
	}))
	defer server.Close()

	profile, err := fetchOAuthProfile(context.Background(), OAuthProviderGitHub, OAuthProviderConfig{
		ClientID: "client-id", ClientSecret: "client-secret", RedirectURL: "http://localhost/callback",
		TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/profile", EmailURL: server.URL + "/emails",
	}, "authorization-code", server.Client())
	if err != nil {
		t.Fatalf("fetch GitHub profile: %v", err)
	}
	if profile.Subject != "12345" || profile.Email != "person@example.com" {
		t.Fatalf("unexpected GitHub profile: %+v", profile)
	}
}

func TestNormalizeOAuthProviderRejectsUnsupportedValues(t *testing.T) {
	provider, err := normalizeOAuthProvider(OAuthProvider(" Google "))
	if err != nil || provider != OAuthProviderGoogle {
		t.Fatalf("expected Google normalization, got %q, %v", provider, err)
	}
	if _, err := normalizeOAuthProvider(OAuthProvider("microsoft")); err != ErrOAuthInvalidProvider {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}
