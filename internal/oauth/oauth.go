package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

// Provider holds endpoints for one OAuth mail vendor.
type Provider struct {
	Key      string
	AuthURL  string
	TokenURL string
	Scopes   string
	IMAPHost string
	IMAPPort int
	SMTPHost string
	SMTPPort int
	SMTPSSL  bool
}

var providers = map[string]Provider{
	"google": {
		Key:      "google",
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		Scopes:   "openid email https://mail.google.com/",
		IMAPHost: "imap.gmail.com", IMAPPort: 993,
		SMTPHost: "smtp.gmail.com", SMTPPort: 465, SMTPSSL: true,
	},
	"microsoft": {
		Key:      "microsoft",
		AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		Scopes:   "openid email offline_access https://outlook.office.com/IMAP.AccessAsUser.All https://outlook.office.com/SMTP.Send",
		IMAPHost: "outlook.office365.com", IMAPPort: 993,
		SMTPHost: "smtp.office365.com", SMTPPort: 587, SMTPSSL: false,
	},
}

// Providers returns the supported vendor keys.
func Providers() []string { return []string{"google", "microsoft"} }

// Compile-time defaults, baked in with:
//
//	go build -ldflags "-X emailclient/internal/oauth.defaultGoogleID=XXX -X emailclient/internal/oauth.defaultMicrosoftID=YYY"
//
// (the app owner registers one OAuth app per vendor; end users never see IDs).
var (
	defaultGoogleID    string
	defaultMicrosoftID string
)

// DefaultClientID resolves app-owned ID: manual arg > build flag > env (dev).
func DefaultClientID(providerKey, manual string) string {
	if strings.TrimSpace(manual) != "" {
		return strings.TrimSpace(manual)
	}
	switch strings.ToLower(providerKey) {
	case "google":
		if defaultGoogleID != "" {
			return defaultGoogleID
		}
		return strings.TrimSpace(env("EMAILCLIENT_GOOGLE_CLIENT_ID"))
	case "microsoft":
		if defaultMicrosoftID != "" {
			return defaultMicrosoftID
		}
		return strings.TrimSpace(env("EMAILCLIENT_MICROSOFT_CLIENT_ID"))
	}
	return ""
}

func env(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func GetProvider(key string) (Provider, bool) {
	p, ok := providers[strings.ToLower(key)]
	return p, ok
}

type pending struct {
	provider Provider
	clientID string
	verifier string
	state    string
	listener net.Listener
	codeCh   chan string
}

var (
	mu      sync.Mutex
	current *pending
)

func randB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// StartFlow opens a loopback redirect and returns the browser URL (PKCE, no secret).
// clientID may be "" when the app ships with built-in IDs.
func StartFlow(providerKey, clientID string) (string, error) {
	p, ok := GetProvider(providerKey)
	if !ok {
		return "", fmt.Errorf("unknown provider %q", providerKey)
	}
	clientID = DefaultClientID(providerKey, clientID)
	if clientID == "" {
		return "", fmt.Errorf("this build is not registered with %s yet (app owner: rebuild with -ldflags, see README)", p.Key)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	verifier := randB64(64)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randB64(16)
	redirect := fmt.Sprintf("http://%s/callback", ln.Addr().String())

	mu.Lock()
	current = &pending{provider: p, clientID: strings.TrimSpace(clientID), verifier: verifier, state: state, listener: ln, codeCh: make(chan string, 1)}
	mu.Unlock()

	q := url.Values{}
	q.Set("client_id", strings.TrimSpace(clientID))
	q.Set("redirect_uri", redirect)
	q.Set("response_type", "code")
	q.Set("scope", p.Scopes)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	return p.AuthURL + "?" + q.Encode(), nil
}

// FinishFlow waits for the browser callback, exchanges the code, stores
// tokens in the keychain, and returns the account email.
func FinishFlow(timeout time.Duration) (string, error) {
	mu.Lock()
	p := current
	mu.Unlock()
	if p == nil {
		return "", fmt.Errorf("no sign-in started")
	}
	defer func() {
		_ = p.listener.Close()
		mu.Lock()
		current = nil
		mu.Unlock()
	}()
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	go func() {
		_ = http.Serve(p.listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/callback" {
				http.NotFound(w, r)
				return
			}
			if r.URL.Query().Get("state") != p.state {
				http.Error(w, "state mismatch", 400)
				return
			}
			select {
			case p.codeCh <- r.URL.Query().Get("code"):
			default:
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<h3>Signed in — you can close this window.</h3>"))
		}))
	}()
	var code string
	select {
	case code = <-p.codeCh:
	case <-time.After(timeout):
		return "", fmt.Errorf("timed out waiting for browser sign-in")
	}
	if code == "" {
		return "", fmt.Errorf("no code returned (access denied?)")
	}
	tok, err := exchange(p, code)
	if err != nil {
		return "", err
	}
	email := emailFromIDToken(tok.IDToken)
	if email == "" {
		return "", fmt.Errorf("could not read account email from sign-in")
	}
	saveTokens(email, p.provider.Key, p.clientID, tok)
	return email, nil
}

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

func exchange(p *pending, code string) (*tokenResp, error) {
	redirect := ""
	if addr := p.listener.Addr(); addr != nil {
		redirect = fmt.Sprintf("http://%s/callback", addr.String())
	}
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirect)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", p.verifier)
	req, _ := http.NewRequest("POST", p.provider.TokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange failed: %s", strings.TrimSpace(string(body)))
	}
	var tok tokenResp
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, err
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token exchange returned no access token")
	}
	return &tok, nil
}

// emailFromIDToken decodes the JWT payload (account routing only, not auth).
func emailFromIDToken(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		UPN               string `json:"upn"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	for _, v := range []string{claims.Email, claims.PreferredUsername, claims.UPN} {
		if strings.Contains(v, "@") {
			return strings.ToLower(v)
		}
	}
	return ""
}

func key(parts ...string) string { return strings.Join(parts, ":") }

// GetValidToken returns a fresh access token, refreshing silently when needed.
func GetValidToken(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	access, err := keyring.Get("emailclient", key("oauth-access", email))
	if err != nil || access == "" {
		return "", fmt.Errorf("no OAuth sign-in for %s", email)
	}
	expStr, _ := keyring.Get("emailclient", key("oauth-expiry", email))
	var exp int64
	_, _ = fmt.Sscanf(expStr, "%d", &exp)
	if time.Now().Unix() < exp-60 {
		return access, nil
	}
	// refresh
	clientInfo, err := keyring.Get("emailclient", key("oauth-client", email))
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(clientInfo, "|", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("bad stored OAuth client")
	}
	p, ok := GetProvider(parts[0])
	if !ok {
		return "", fmt.Errorf("unknown provider")
	}
	refresh, err := keyring.Get("emailclient", key("oauth-refresh", email))
	if err != nil || refresh == "" {
		return "", fmt.Errorf("sign-in expired, please sign in again")
	}
	form := url.Values{}
	form.Set("client_id", parts[1])
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	req, _ := http.NewRequest("POST", p.TokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("refresh failed, please sign in again")
	}
	var tok tokenResp
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("refresh failed, please sign in again")
	}
	if tok.RefreshToken != "" {
		refresh = tok.RefreshToken
	}
	saveTokens(email, p.Key, parts[1], &tokenResp{AccessToken: tok.AccessToken, RefreshToken: refresh, ExpiresIn: tok.ExpiresIn})
	return tok.AccessToken, nil
}

func saveTokens(email, provider, clientID string, tok *tokenResp) {
	exp := time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second).Unix()
	if tok.ExpiresIn <= 0 {
		exp = time.Now().Add(50 * time.Minute).Unix()
	}
	_ = keyring.Set("emailclient", key("oauth-access", email), tok.AccessToken)
	if tok.RefreshToken != "" {
		_ = keyring.Set("emailclient", key("oauth-refresh", email), tok.RefreshToken)
	}
	_ = keyring.Set("emailclient", key("oauth-expiry", email), fmt.Sprintf("%d", exp))
	_ = keyring.Set("emailclient", key("oauth-client", email), provider+"|"+clientID)
}

// DeleteTokens removes all OAuth secrets for an account.
func DeleteTokens(email string) {
	for _, k := range []string{"oauth-access", "oauth-refresh", "oauth-expiry", "oauth-client"} {
		_ = keyring.Delete("emailclient", key(k, strings.ToLower(email)))
	}
}

// ProviderFor returns the vendor config for an OAuth-signed account.
func ProviderFor(email string) (Provider, bool) {
	info, err := keyring.Get("emailclient", key("oauth-client", strings.ToLower(strings.TrimSpace(email))))
	if err != nil {
		return Provider{}, false
	}
	parts := strings.SplitN(info, "|", 2)
	if len(parts) != 2 {
		return Provider{}, false
	}
	return GetProvider(parts[0])
}
