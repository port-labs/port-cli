package auth

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/cli/browser"
	"github.com/golang-jwt/jwt/v5"
	"github.com/port-labs/port-cli/internal/styles"
	"golang.org/x/oauth2"
)

func ReadTokenFromStdin() (string, error) {
	return ReadToken(os.Stdin)
}

func ReadToken(f fs.File) (string, error) {
	stat, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("failed reading token (%w)", err)
	}

	if stat.Mode()&os.ModeNamedPipe == 0 && stat.Size() == 0 {
		return "", fmt.Errorf("no token provided")
	}
	reader := bufio.NewReader(f)
	var b strings.Builder
	for {
		r, _, rErr := reader.ReadRune()
		if rErr != nil && rErr == io.EOF {
			break
		}
		_, rErr = b.WriteRune(r)
		if rErr != nil {
			return "", fmt.Errorf("error getting input (%w)", rErr)
		}
	}

	return b.String(), nil
}

type LoginOpts struct {
	BaseURL string
	APIURL  string
	Org     string
}

var clientIds = map[string]string{
	"https://auth.getport.io":         "DEcppuFTwCgBDGxgD2sOyJ0xOQx3p2OP",
	"https://auth.us.getport.io":      "OWZg1272IgNmjz7PPYP9bk7K3pzZkIeM",
	"https://auth.staging.getport.io": "bY90kSHEuHEmQy6vtABmoQITeH4N6SFA",
	"http://api.localhost:9080":       "dAea4bpVXnr0ohLCdLKWgIgtC22sSSWl",
}

func registerClientID(baseURL, clientID string) {
	clientIds[baseURL] = clientID
}

func unregisterClientID(baseURL string) {
	delete(clientIds, baseURL)
}

// refreshClient is used exclusively for token refresh calls.
// The short timeout ensures a stale Auth0 endpoint never blocks CLI commands
// indefinitely at startup.
var refreshClient = &http.Client{Timeout: 10 * time.Second}

var ErrInterrupted = errors.New("interrupted")

func TokenFromOAuth(ctx context.Context, opts LoginOpts) (*Token, error) {
	obtainedToken := make(chan *oauth2.Token)

	clientId, ok := clientIds[opts.BaseURL]
	if !ok {
		return nil, fmt.Errorf("base url %s is not supported", opts.BaseURL)
	}

	conf := &oauth2.Config{
		ClientID:    clientId,
		RedirectURL: "http://localhost:4321/oauth/callback",
		Scopes:      []string{"openid", "offline_access"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  fmt.Sprintf("%s/authorize?audience=%s", opts.BaseURL, opts.APIURL),
			TokenURL: fmt.Sprintf("%s/oauth/token", opts.BaseURL),
		},
	}

	verifier := oauth2.GenerateVerifier()

	handler := func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write(bytes.NewBufferString("There was an error authenticating, please try again.").Bytes())
			obtainedToken <- nil
			return
		}

		token, err := conf.Exchange(
			ctx,
			code,
			oauth2.VerifierOption(verifier),
		)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal error. Authentication failed.\n"))
			obtainedToken <- nil
			return
		}

		obtainedToken <- token

		w.Header().Set("Content-Type", "text/html")
		w.Write(bytes.NewBufferString("Logged in successfully. You can now close this window.").Bytes())
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", handler)
	server := &http.Server{Handler: mux}
	listener, err := net.Listen("tcp", "127.0.0.1:4321")
	if err != nil {
		return nil, fmt.Errorf("failed to start local auth callback server: %w", err)
	}
	serverErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()
	defer server.Shutdown(ctx)

	lipgloss.Printf("Opening a browser to log you into %s...\n", styles.Bold.Render(opts.Org))

	url := conf.AuthCodeURL("state", oauth2.S256ChallengeOption(verifier))
	err = browser.OpenURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed opening a browser")
	}

	var token *oauth2.Token
	var wg sync.WaitGroup
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	wg.Add(1)
	go func() {
		for {
			select {
			case t := <-obtainedToken:
				token = t
				wg.Done()
				return

			case serveErr := <-serverErr:
				if serveErr != nil {
					err = fmt.Errorf("auth callback server failed: %w", serveErr)
					wg.Done()
					return
				}
			case <-interrupt:
				err = ErrInterrupted
				wg.Done()
				return
			}
		}
	}()
	wg.Wait()

	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, fmt.Errorf("failed logging in")
	}

	parsed, err := ParseToken(token.AccessToken)
	if err != nil {
		return nil, err
	}
	parsed.RefreshToken = token.RefreshToken
	parsed.AuthBaseURL = opts.BaseURL
	return parsed, nil
}

type Claims struct {
	Audience string    `json:"aud"`
	OrgName  string    `json:"orgName"`
	OrgId    string    `json:"orgId"`
	Email    string    `json:"email"`
	Expiry   time.Time `json:"exp"`
}
type Token struct {
	Token        string
	Claims       Claims
	RefreshToken string `json:"refresh_token,omitempty"`
	AuthBaseURL  string `json:"auth_base_url,omitempty"`
}

func ParseToken(token string) (*Token, error) {
	claims := jwt.MapClaims{}
	t, _, err := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()})).ParseUnverified(token, &claims)
	if err != nil {
		return nil, err
	}
	aud, err := t.Claims.GetAudience()
	if err != nil {
		return nil, err
	}
	if len(aud) == 0 {
		return nil, fmt.Errorf("missing audience in token")
	}

	emailKey := fmt.Sprintf("%s/email", aud[0])
	email, found := claims[emailKey]
	if !found {
		return nil, fmt.Errorf("failed finding email claim")
	}
	if _, ok := email.(string); !ok {
		return nil, fmt.Errorf("email claim is not a string")
	}

	orgIdKey := fmt.Sprintf("%s/orgId", aud[0])
	orgId, found := claims[orgIdKey]
	if !found {
		return nil, fmt.Errorf("failed finding orgId claim")
	}
	if _, ok := orgId.(string); !ok {
		return nil, fmt.Errorf("orgId claim is not a string")
	}

	orgNameKey := fmt.Sprintf("%s/orgName", aud[0])
	orgName, found := claims[orgNameKey]
	if !found {
		return nil, fmt.Errorf("failed finding orgName claim")
	}
	if _, ok := orgName.(string); !ok {
		return nil, fmt.Errorf("orgName claim is not a string")
	}

	exp, found := claims["exp"]
	if !found {
		return nil, fmt.Errorf("failed finding exp claim")
	}
	if _, ok := exp.(float64); !ok {
		return nil, fmt.Errorf("exp claim is not a float64")
	}
	expiry := int64(exp.(float64))

	return &Token{
		Token: t.Raw,
		Claims: Claims{
			Audience: aud[0],
			Email:    email.(string),
			OrgId:    orgId.(string),
			OrgName:  orgName.(string),
			Expiry:   time.Unix(expiry, 0),
		},
	}, err
}

// RefreshAccessToken exchanges a refresh token for a new access token.
// It is a package-level variable so tests can replace it with a stub without
// needing to manipulate the internal clientIds map.
var RefreshAccessToken = refreshAccessToken

func refreshAccessToken(ctx context.Context, authBaseURL, oldRefreshToken string) (*Token, error) {
	clientID, ok := clientIds[authBaseURL]
	if !ok {
		return nil, fmt.Errorf("base url %s is not supported", authBaseURL)
	}

	// Refresh tokens are opaque to the CLI, so we cannot inspect their expiry
	// locally. Auth0 validates whether the refresh token is still usable when we
	// attempt the exchange below.
	payload, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     clientID,
		"refresh_token": oldRefreshToken,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refresh request (%w)", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/oauth/token", authBaseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed creating refresh request (%w)", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := refreshClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed refreshing token (%w)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed refreshing token (%s): %s", resp.Status, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed decoding refresh response (%w)", err)
	}

	parsed, err := ParseToken(tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed parsing refreshed token (%w)", err)
	}
	parsed.AuthBaseURL = authBaseURL
	if tokenResp.RefreshToken != "" {
		parsed.RefreshToken = tokenResp.RefreshToken
	} else {
		// Some providers only rotate refresh tokens occasionally, so keep the
		// existing one when the response omits a replacement token.
		parsed.RefreshToken = oldRefreshToken
	}

	return parsed, nil
}
