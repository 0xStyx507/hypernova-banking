package auth

import (
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultOAuthStateTTL    = 10 * time.Minute
	defaultOAuthExchangeTTL = 2 * time.Minute
)

// OAuthProvider identifies a supported external identity provider.
type OAuthProvider string

const (
	OAuthProviderGoogle OAuthProvider = "google"
	OAuthProviderGitHub OAuthProvider = "github"
)

// OAuthProviderConfig contains server-side provider configuration. Secrets
// are read from the environment and never serialized to a client.
type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	EmailURL     string
	Scope        string
}

// OAuthConfig controls provider endpoints and short-lived OAuth artifacts.
type OAuthConfig struct {
	Google      OAuthProviderConfig
	GitHub      OAuthProviderConfig
	StateTTL    time.Duration
	ExchangeTTL time.Duration
	HTTPClient  *http.Client
}

type OAuthAuthorization struct {
	Provider  OAuthProvider `json:"provider"`
	URL       string        `json:"url"`
	State     string        `json:"-"`
	ExpiresAt time.Time     `json:"expires_at"`
}

type OAuthCallbackResult struct {
	User         User
	ExchangeCode string
	ExpiresAt    time.Time
	NewUser      bool
}

type oauthProfile struct {
	Subject  string
	Email    string
	FullName string
}

// OAuthConfigFromEnv creates a safe local configuration. Empty credentials
// intentionally leave a provider unavailable instead of enabling a fake flow.
func OAuthConfigFromEnv() OAuthConfig {
	return OAuthConfig{
		Google: OAuthProviderConfig{
			ClientID: os.Getenv("GOOGLE_OAUTH_CLIENT_ID"), ClientSecret: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
			RedirectURL: os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"), AuthURL: "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token", UserInfoURL: "https://openidconnect.googleapis.com/v1/userinfo", Scope: "openid email profile",
		},
		GitHub: OAuthProviderConfig{
			ClientID: os.Getenv("GITHUB_OAUTH_CLIENT_ID"), ClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
			RedirectURL: os.Getenv("GITHUB_OAUTH_REDIRECT_URL"), AuthURL: "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token", UserInfoURL: "https://api.github.com/user",
			EmailURL: "https://api.github.com/user/emails", Scope: "read:user user:email",
		},
		StateTTL: durationFromEnv("OAUTH_STATE_TTL", defaultOAuthStateTTL), ExchangeTTL: durationFromEnv("OAUTH_EXCHANGE_TTL", defaultOAuthExchangeTTL),
		HTTPClient: &http.Client{Timeout: 8 * time.Second},
	}
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func (s *Service) oauthProvider(provider OAuthProvider) (OAuthProviderConfig, error) {
	if s == nil || s.pool == nil {
		return OAuthProviderConfig{}, ErrOAuthNotConfigured
	}
	var config OAuthProviderConfig
	switch provider {
	case OAuthProviderGoogle:
		config = s.oauth.Google
	case OAuthProviderGitHub:
		config = s.oauth.GitHub
	default:
		return OAuthProviderConfig{}, ErrOAuthInvalidProvider
	}
	if strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.ClientSecret) == "" || strings.TrimSpace(config.RedirectURL) == "" || strings.TrimSpace(config.AuthURL) == "" || strings.TrimSpace(config.TokenURL) == "" || strings.TrimSpace(config.UserInfoURL) == "" || (provider == OAuthProviderGitHub && strings.TrimSpace(config.EmailURL) == "") {
		return OAuthProviderConfig{}, ErrOAuthNotConfigured
	}
	return config, nil
}

func normalizeOAuthProvider(provider OAuthProvider) (OAuthProvider, error) {
	provider = OAuthProvider(strings.ToLower(strings.TrimSpace(string(provider))))
	if provider != OAuthProviderGoogle && provider != OAuthProviderGitHub {
		return "", ErrOAuthInvalidProvider
	}
	return provider, nil
}

func (s *Service) oauthStateTTL() time.Duration {
	if s.oauth.StateTTL > 0 {
		return s.oauth.StateTTL
	}
	return defaultOAuthStateTTL
}

func (s *Service) oauthExchangeTTL() time.Duration {
	if s.oauth.ExchangeTTL > 0 {
		return s.oauth.ExchangeTTL
	}
	return defaultOAuthExchangeTTL
}

func (s *Service) oauthHTTPClient() *http.Client {
	if s.oauth.HTTPClient != nil {
		return s.oauth.HTTPClient
	}
	return &http.Client{Timeout: 8 * time.Second}
}
