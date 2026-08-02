package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const maxOAuthResponseBytes = 1 << 20

func fetchOAuthProfile(ctx context.Context, provider OAuthProvider, config OAuthProviderConfig, code string, client *http.Client) (oauthProfile, error) {
	accessToken, err := exchangeProviderCode(ctx, config, code, client)
	if err != nil {
		return oauthProfile{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, config.UserInfoURL, nil)
	if err != nil {
		return oauthProfile{}, ErrOAuthProvider
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return oauthProfile{}, ErrOAuthProvider
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return oauthProfile{}, ErrOAuthProvider
	}
	if provider == OAuthProviderGoogle {
		var profile struct {
			Subject  string `json:"sub"`
			Email    string `json:"email"`
			Verified bool   `json:"email_verified"`
			Name     string `json:"name"`
		}
		if err := decodeOAuthJSON(response, &profile); err != nil || profile.Subject == "" || profile.Email == "" || !profile.Verified {
			return oauthProfile{}, ErrOAuthIdentity
		}
		return oauthProfile{Subject: profile.Subject, Email: profile.Email, FullName: profile.Name}, nil
	}
	var profile struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := decodeOAuthJSON(response, &profile); err != nil || profile.ID == 0 {
		return oauthProfile{}, ErrOAuthIdentity
	}
	// GitHub's /user response does not prove that an email is verified. Always
	// resolve the primary verified address from /user/emails before linking it.
	verifiedEmail, err := fetchGitHubVerifiedEmail(ctx, config, accessToken, client)
	if err != nil {
		return oauthProfile{}, err
	}
	return oauthProfile{Subject: strconv.FormatInt(profile.ID, 10), Email: verifiedEmail, FullName: firstNonEmpty(profile.Name, profile.Login)}, nil
}

func exchangeProviderCode(ctx context.Context, config OAuthProviderConfig, code string, client *http.Client) (string, error) {
	form := url.Values{"client_id": {config.ClientID}, "client_secret": {config.ClientSecret}, "code": {code}, "redirect_uri": {config.RedirectURL}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", ErrOAuthProvider
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", ErrOAuthProvider
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", ErrOAuthProvider
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := decodeOAuthJSON(response, &token); err != nil || token.AccessToken == "" {
		return "", ErrOAuthProvider
	}
	return token.AccessToken, nil
}

func fetchGitHubVerifiedEmail(ctx context.Context, config OAuthProviderConfig, accessToken string, client *http.Client) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, config.EmailURL, nil)
	if err != nil {
		return "", ErrOAuthProvider
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(request)
	if err != nil {
		return "", ErrOAuthProvider
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", ErrOAuthProvider
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := decodeOAuthJSON(response, &emails); err != nil {
		return "", ErrOAuthProvider
	}
	for _, email := range emails {
		if email.Primary && email.Verified && strings.TrimSpace(email.Email) != "" {
			return email.Email, nil
		}
	}
	return "", ErrOAuthIdentity
}

func decodeOAuthJSON(response *http.Response, target any) error {
	if response == nil || response.Body == nil {
		return errors.New("empty oauth response")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthResponseBytes))
	if err != nil {
		return fmt.Errorf("read oauth response: %w", err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode oauth response: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "Hypernova user"
}

// newUnusablePasswordHash gives an OAuth-only identity a valid database hash
// without creating a password that the user or the provider can know.
func newUnusablePasswordHash(cost int) (string, error) {
	return HashPassword("oauth-disabled-"+uuid.NewString(), cost)
}
