package transitions

// Social sign-in (Google, Microsoft, Apple, LinkedIn) - Authorization Code
// flow, no client-side SDKs. A consuming project's frontend fetches a
// ready-made authorize URL from BuildAuthorizeURL, navigates the whole page
// there, and every provider redirects back to the SAME frontend route
// (FRONTEND_URL/oauth/callback/{provider}) with a plain `?code=&state=`
// query string - the frontend then POSTs {provider, code, state} to a
// workflow that calls CompleteExchange below, gets {email, name} back, and
// decides what to do with it (look up or create an account, mint a
// session token) - that part is deliberately NOT this package's job. This
// package only ever proves "this person really does control this email
// address at this provider," nothing about what your application's
// accounts look like.
//
// No Redis (or any other storage) dependency at all - the one-time state
// token this package generates is just a random string; persisting it
// (and validating/consuming it again on the way back) is the calling
// project's job, via whatever storage it already uses. See this repo's
// workflows/oauth/*.wsl for the reference shape: build_authorize_url.wsl
// stores the state via redis/string.Set right after calling
// BuildAuthorizeURL, complete_exchange.wsl consumes it via
// redis/string.GetDel and checks it matches the claimed provider BEFORE
// ever calling CompleteExchange - a consuming project is free to use a
// different store entirely, this package never assumes Redis exists.
//
// net/http is used directly for the provider HTTP calls - the first
// HTTP-calling-out code in the kuetix package ecosystem as far as this
// package is concerned, no new dependency needed.

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

const defaultFrontendURL = "http://localhost:5173"

type oauthTransitions struct {
	workflow.BaseServiceTransition
}

func NewOAuthTransitions() interfaces.ServiceTransitions {
	return &oauthTransitions{}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func frontendURL() string {
	if v := os.Getenv("OAUTH_REDIRECT_BASE"); v != "" {
		return v
	}
	return envOr("FRONTEND_URL", defaultFrontendURL)
}

func redirectURI(provider string) string {
	return frontendURL() + "/oauth/callback/" + provider
}

// resolveRedirectURI lets a caller (e.g. a native app with no meaningful
// FRONTEND_URL to redirect back to) supply its own redirect_uri - a custom
// URL scheme registered with the provider, typically - instead of the
// env-derived web default. Empty override falls back to redirectURI
// unchanged, so every existing web caller is unaffected. The caller must
// pass the SAME value (or none) to both BuildAuthorizeURL and
// CompleteExchange for a given sign-in attempt - OAuth requires the
// redirect_uri sent during token exchange to exactly match the one used
// to obtain the authorization code, and the provider itself is what
// enforces that; this package doesn't need to remember or validate it
// independently.
func resolveRedirectURI(provider, override string) string {
	if override != "" {
		return override
	}
	return redirectURI(provider)
}

// generateState returns a random, URL-safe one-time token - CSRF/replay
// protection for the round trip through the provider's consent screen and
// back. See this file's header comment: persisting and later validating
// it is the calling project's job, not this package's.
func generateState() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ---- authorize URLs --------------------------------------------------------
//
// Google/Microsoft/LinkedIn all use response_mode=query (the default) - a
// plain ?code=&state= redirect that one shared frontend route can consume
// directly. Apple is the exception: confirmed against real Apple behavior
// (not just its docs), Apple hard-requires response_mode=form_post whenever
// scope includes name/email - a bare response_type=code request with that
// scope and no response_mode 500s on Apple's own server. Since
// exchangeApple below treats a missing email as fatal, scope can't be
// dropped to dodge this, which means Apple's redirectURI must point at
// something able to receive a POST and bridge it back to a query-string
// redirect for the shared frontend route (see this project's
// cmd/api/apple_bridge.go) - Google/Microsoft/LinkedIn's redirectURI can
// still point at the frontend route directly.
func buildAuthorizeURL(provider, state, redirectURI string) (string, error) {
	switch provider {
	case "google":
		clientID := os.Getenv("GOOGLE_CLIENT_ID")
		if clientID == "" {
			return "", fmt.Errorf("GOOGLE_CLIENT_ID is not configured")
		}
		v := url.Values{
			"client_id":     {clientID},
			"redirect_uri":  {redirectURI},
			"response_type": {"code"},
			"scope":         {"openid email profile"},
			"state":         {state},
			"prompt":        {"select_account"},
		}
		return "https://accounts.google.com/o/oauth2/v2/auth?" + v.Encode(), nil

	case "microsoft":
		clientID := os.Getenv("MICROSOFT_CLIENT_ID")
		if clientID == "" {
			return "", fmt.Errorf("MICROSOFT_CLIENT_ID is not configured")
		}
		tenant := envOr("MICROSOFT_TENANT", "common")
		v := url.Values{
			"client_id":     {clientID},
			"redirect_uri":  {redirectURI},
			"response_type": {"code"},
			"scope":         {"openid email profile"},
			"state":         {state},
		}
		return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize?%s", url.PathEscape(tenant), v.Encode()), nil

	case "linkedin":
		clientID := os.Getenv("LINKEDIN_CLIENT_ID")
		if clientID == "" {
			return "", fmt.Errorf("LINKEDIN_CLIENT_ID is not configured")
		}
		v := url.Values{
			"client_id":     {clientID},
			"redirect_uri":  {redirectURI},
			"response_type": {"code"},
			"scope":         {"openid email profile"},
			"state":         {state},
		}
		return "https://www.linkedin.com/oauth/v2/authorization?" + v.Encode(), nil

	case "apple":
		clientID := os.Getenv("APPLE_CLIENT_ID")
		if clientID == "" {
			return "", fmt.Errorf("APPLE_CLIENT_ID is not configured")
		}
		// Apple hard-requires response_mode=form_post whenever scope
		// includes name/email (confirmed against real Apple behavior - a
		// bare response_type=code request with this scope 500s on Apple's
		// side without it, contrary to this file's older header comment).
		// That means code/state arrive at redirectURI as a POST body, not
		// a query string - the caller is responsible for pointing
		// redirectURI at something that can actually receive a POST and
		// bridge it back to a query-string redirect for a plain frontend
		// route to consume (see this project's cmd/api/apple_bridge.go).
		v := url.Values{
			"client_id":     {clientID},
			"redirect_uri":  {redirectURI},
			"response_type": {"code"},
			"response_mode": {"form_post"},
			"scope":         {"name email"},
			"state":         {state},
		}
		return "https://appleid.apple.com/auth/authorize?" + v.Encode(), nil

	default:
		return "", fmt.Errorf("unknown provider %q", provider)
	}
}

// BuildAuthorizeURL never sets r.Error, even on failure - always
// r.Success = true, with the actual outcome (ok/error/url/state) riding in
// the response for the calling workflow to branch on via assert.Bool. This
// engine treats any r.Error as fatal and skips the calling workflow's own
// `on fail ->` transition entirely, a confirmed quirk with actions that use
// the "set r.Error on failure" convention - see CompleteExchange's
// identical comment.
//
// redirectURIOverride is empty for the web flow (falls back to the
// FRONTEND_URL-derived default - see resolveRedirectURI); a native app
// with its own registered custom URL scheme passes it explicitly, and
// must pass the identical value to CompleteExchange for the same sign-in
// attempt.
func (t *oauthTransitions) BuildAuthorizeURL(provider, redirectURIOverride string) (r domain.FlowStepResult) {
	r.Success = true
	r.StatusCode = http.StatusOK

	state, err := generateState()
	if err != nil {
		r.Response = map[string]interface{}{"ok": false, "error": "failed to start sign-in"}
		return
	}

	authorizeURL, err := buildAuthorizeURL(provider, state, resolveRedirectURI(provider, redirectURIOverride))
	if err != nil {
		r.Response = map[string]interface{}{"ok": false, "error": err.Error()}
		return
	}

	// Persisting `state` is the calling workflow's job, not this action's -
	// see this file's header comment.
	r.Response = map[string]interface{}{"ok": true, "url": authorizeURL, "state": state}
	return
}

// ---- token exchange + profile fetch ---------------------------------------

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

func postForm(ctx context.Context, tokenURL string, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("could not reach the sign-in provider: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return tokenResponse{}, err
	}

	var out tokenResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return tokenResponse{}, fmt.Errorf("unexpected response from sign-in provider")
	}
	if out.Error != "" {
		msg := out.Error
		if out.ErrorDesc != "" {
			msg = out.ErrorDesc
		}
		return tokenResponse{}, fmt.Errorf("sign-in provider rejected the request: %s", msg)
	}
	if resp.StatusCode != http.StatusOK {
		return tokenResponse{}, fmt.Errorf("sign-in provider returned status %d", resp.StatusCode)
	}
	return out, nil
}

type oidcUserInfo struct {
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"` // Google: bool, some providers: "true"/"false" string
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	Mail          string `json:"mail"` // Microsoft Graph fallback shape, unused via /oidc/userinfo but harmless to keep
}

// exchangeOIDC handles the three providers that expose a standard OIDC
// /userinfo endpoint (Google, Microsoft, LinkedIn) - trade the code for an
// access_token, then use that token to fetch email/name directly, rather
// than decoding an id_token ourselves for all three (only Apple forces
// that - see exchangeApple).
func exchangeOIDC(ctx context.Context, tokenURL, userinfoURL, clientID, clientSecret, code, redirectURI string) (email, name string, err error) {
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}
	tok, err := postForm(ctx, tokenURL, form)
	if err != nil {
		return "", "", err
	}
	if tok.AccessToken == "" {
		return "", "", fmt.Errorf("sign-in provider did not return an access token")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("could not reach the sign-in provider: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}

	var info oidcUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return "", "", fmt.Errorf("unexpected profile response from sign-in provider")
	}
	email = info.Email
	if email == "" {
		email = info.Mail
	}
	if email == "" {
		return "", "", fmt.Errorf("sign-in provider did not share an email address")
	}
	name = info.Name
	if name == "" {
		name = info.GivenName
	}
	return email, name, nil
}

// ---- Apple ------------------------------------------------------------
//
// Apple has no /userinfo endpoint at all - the only place user info ever
// appears is the id_token JWT that comes back from the token exchange
// (name is a rare exception: sent once, as a JSON blob in the ORIGINAL
// form_post callback body, never in the token response or on any later
// sign-in - see buildAuthorizeURL's comment on why this package doesn't
// try to capture it). Apple also requires the client_secret itself to be
// a freshly-signed ES256 JWT (APPLE_PRIVATE_KEY, a .p8 key Apple issues),
// not a static string like the other three providers.

func appleClientSecret() (string, error) {
	teamID := os.Getenv("APPLE_TEAM_ID")
	keyID := os.Getenv("APPLE_KEY_ID")
	clientID := os.Getenv("APPLE_CLIENT_ID")
	// .p8 key contents - env vars commonly can't hold real newlines, so a
	// literal "\n" is accepted and unescaped here (matching how most
	// platforms document pasting a PEM block into a single-line env var).
	privateKeyPEM := strings.ReplaceAll(os.Getenv("APPLE_PRIVATE_KEY"), `\n`, "\n")
	if teamID == "" || keyID == "" || clientID == "" || privateKeyPEM == "" {
		return "", fmt.Errorf("Apple sign-in is not configured (set APPLE_TEAM_ID, APPLE_KEY_ID, APPLE_CLIENT_ID, APPLE_PRIVATE_KEY)")
	}

	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("APPLE_PRIVATE_KEY is not a valid PEM-encoded key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("APPLE_PRIVATE_KEY could not be parsed: %w", err)
	}
	ecKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("APPLE_PRIVATE_KEY must be the EC private key from Apple's .p8 file")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": teamID,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"aud": "https://appleid.apple.com",
		"sub": clientID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = keyID
	return token.SignedString(ecKey)
}

type appleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

var (
	appleJWKSOnce sync.Once
	appleJWKSKeys []appleJWK
	appleJWKSErr  error
)

// Fetched once per process (Apple rotates signing keys rarely; a restart
// naturally picks up new ones) - a deliberately simple, non-refreshing
// cache rather than a TTL-based one.
func fetchAppleJWKS(ctx context.Context) ([]appleJWK, error) {
	appleJWKSOnce.Do(func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://appleid.apple.com/auth/keys", nil)
		if err != nil {
			appleJWKSErr = err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			appleJWKSErr = err
			return
		}
		defer resp.Body.Close()
		var out struct {
			Keys []appleJWK `json:"keys"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			appleJWKSErr = err
			return
		}
		appleJWKSKeys = out.Keys
	})
	return appleJWKSKeys, appleJWKSErr
}

func rsaPublicKeyFromJWK(k appleJWK) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

// verifyAppleIDToken checks the id_token's signature against Apple's
// published JWKS and confirms it was actually issued for our own
// APPLE_CLIENT_ID - without this, anyone could hand us a validly-formed
// but unrelated Apple id_token and have it accepted as proof of an email.
func verifyAppleIDToken(ctx context.Context, idToken string) (email string, err error) {
	keys, err := fetchAppleJWKS(ctx)
	if err != nil {
		return "", fmt.Errorf("could not verify Apple sign-in: %w", err)
	}

	token, err := jwt.Parse(idToken, func(t *jwt.Token) (interface{}, error) {
		kid, _ := t.Header["kid"].(string)
		for _, k := range keys {
			if k.Kid == kid {
				return rsaPublicKeyFromJWK(k)
			}
		}
		return nil, fmt.Errorf("no matching Apple signing key for kid %q", kid)
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil || !token.Valid {
		return "", fmt.Errorf("Apple sign-in token could not be verified")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("Apple sign-in token had an unexpected shape")
	}
	if aud, _ := claims["aud"].(string); aud != os.Getenv("APPLE_CLIENT_ID") {
		return "", fmt.Errorf("Apple sign-in token was not issued for this app")
	}
	email, _ = claims["email"].(string)
	if email == "" {
		return "", fmt.Errorf("Apple did not share an email address")
	}
	return email, nil
}

func exchangeApple(ctx context.Context, code, redirectURI string) (email, name string, err error) {
	clientSecret, err := appleClientSecret()
	if err != nil {
		return "", "", err
	}
	form := url.Values{
		"client_id":     {os.Getenv("APPLE_CLIENT_ID")},
		"client_secret": {clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}
	tok, err := postForm(ctx, "https://appleid.apple.com/auth/token", form)
	if err != nil {
		return "", "", err
	}
	if tok.IDToken == "" {
		return "", "", fmt.Errorf("Apple did not return a sign-in token")
	}
	email, err = verifyAppleIDToken(ctx, tok.IDToken)
	if err != nil {
		return "", "", err
	}
	return email, "", nil // see this file's Apple section comment: no name available via this flow
}

func exchangeAndFetchProfile(ctx context.Context, provider, code, redirectURI string) (email, name string, err error) {
	switch provider {
	case "google":
		clientID, clientSecret := os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET")
		if clientID == "" || clientSecret == "" {
			return "", "", fmt.Errorf("Google sign-in is not configured")
		}
		return exchangeOIDC(ctx, "https://oauth2.googleapis.com/token", "https://openidconnect.googleapis.com/v1/userinfo", clientID, clientSecret, code, redirectURI)

	case "microsoft":
		clientID, clientSecret := os.Getenv("MICROSOFT_CLIENT_ID"), os.Getenv("MICROSOFT_CLIENT_SECRET")
		if clientID == "" || clientSecret == "" {
			return "", "", fmt.Errorf("Microsoft sign-in is not configured")
		}
		tenant := envOr("MICROSOFT_TENANT", "common")
		tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(tenant))
		return exchangeOIDC(ctx, tokenURL, "https://graph.microsoft.com/oidc/userinfo", clientID, clientSecret, code, redirectURI)

	case "linkedin":
		clientID, clientSecret := os.Getenv("LINKEDIN_CLIENT_ID"), os.Getenv("LINKEDIN_CLIENT_SECRET")
		if clientID == "" || clientSecret == "" {
			return "", "", fmt.Errorf("LinkedIn sign-in is not configured")
		}
		return exchangeOIDC(ctx, "https://www.linkedin.com/oauth/v2/accessToken", "https://api.linkedin.com/v2/userinfo", clientID, clientSecret, code, redirectURI)

	case "apple":
		return exchangeApple(ctx, code, redirectURI)

	default:
		return "", "", fmt.Errorf("unknown provider %q", provider)
	}
}

// CompleteExchange trades `code` for the signed-in user's email (and name,
// where the provider offers one). The caller must already have verified
// the one-time state token before calling this (see this file's header
// comment) - this action doesn't know about state at all. Never sets
// r.Error - see BuildAuthorizeURL's identical comment. Deliberately stops
// here: what a consuming project does with {email, name} - look up or
// create its own account, mint its own session token - is that project's
// job, not this package's.
//
// redirectURIOverride must be the exact same value (or the exact same
// absence of one) passed to BuildAuthorizeURL for this sign-in attempt -
// see resolveRedirectURI's doc comment on why the provider itself is what
// enforces that, not this package.
func (t *oauthTransitions) CompleteExchange(provider, code, redirectURIOverride string) (r domain.FlowStepResult) {
	r.Success = true
	r.StatusCode = http.StatusOK

	email, name, err := exchangeAndFetchProfile(context.Background(), provider, code, resolveRedirectURI(provider, redirectURIOverride))
	if err != nil {
		r.Response = map[string]interface{}{"ok": false, "error": err.Error()}
		return
	}

	r.Response = map[string]interface{}{"ok": true, "email": email, "name": name}
	return
}
