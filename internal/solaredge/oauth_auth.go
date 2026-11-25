package solaredge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const authorizeURL = "https://connect.solaredge.com/authorize"

// Configured reports whether client id/secret are present (auth can start).
func (c *OAuthClient) Configured() bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.opts.ClientID) != "" && strings.TrimSpace(c.opts.ClientSecret) != ""
}

// AuthStatus is a redacted snapshot for the settings UI.
type AuthStatus struct {
	Configured bool   `json:"configured"`
	Authorized bool   `json:"authorized"`
	SiteID     string `json:"site_id"`
	TokenPath  string `json:"token_path,omitempty"`
}

// Status returns connection state without secrets.
func (c *OAuthClient) Status() AuthStatus {
	if c == nil {
		return AuthStatus{}
	}
	return AuthStatus{
		Configured: c.Configured(),
		Authorized: strings.TrimSpace(c.refreshToken()) != "" && strings.TrimSpace(c.siteID()) != "",
		SiteID:     c.siteID(),
		TokenPath:  c.opts.TokenPath,
	}
}

// AuthorizeURL builds the SolarEdge Site Access consent URL.
func (c *OAuthClient) AuthorizeURL(redirectURI, state string) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("solaredge oauth: client id/secret not configured")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return "", fmt.Errorf("solaredge oauth: redirect_uri required")
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", strings.TrimSpace(c.opts.ClientID))
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "SITE_DATA DEVICE_DATA")
	q.Set("access_duration", "1051200")
	q.Set("external_id", "kindle-shazam")
	if state != "" {
		q.Set("state", state)
	}
	return authorizeURL + "?" + q.Encode(), nil
}

// ExchangeCode swaps an authorization code for tokens and persists them.
func (c *OAuthClient) ExchangeCode(ctx context.Context, code, redirectURI, siteID string) error {
	if !c.Configured() {
		return fmt.Errorf("solaredge oauth: client id/secret not configured")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("solaredge oauth: empty authorization code")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return fmt.Errorf("solaredge oauth: redirect_uri required")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	basic := base64.StdEncoding.EncodeToString([]byte(c.opts.ClientID + ":" + c.opts.ClientSecret))
	req.Header.Set("Authorization", "Basic "+basic)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("solaredge code exchange: %s: %s", res.Status, truncate(string(body), 240))
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return err
	}
	if out.AccessToken == "" || out.RefreshToken == "" {
		return fmt.Errorf("solaredge code exchange: missing tokens")
	}
	exp := time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	if out.ExpiresIn <= 0 {
		exp = time.Now().Add(2 * time.Hour)
	}

	site := strings.TrimSpace(siteID)
	if site == "" {
		site = c.siteID()
	}

	c.mu.Lock()
	c.token.AccessToken = out.AccessToken
	c.token.RefreshToken = out.RefreshToken
	c.token.ExpiresAt = exp
	if site != "" {
		c.token.SiteID = site
		c.opts.SiteID = site
	}
	path := c.opts.TokenPath
	tf := c.token
	c.mu.Unlock()
	if err := saveTokenFile(path, tf); err != nil {
		return fmt.Errorf("solaredge save token: %w", err)
	}
	return nil
}

// SetSiteID updates the active site id and persists it with existing tokens.
func (c *OAuthClient) SetSiteID(siteID string) error {
	if c == nil {
		return fmt.Errorf("solaredge oauth: nil client")
	}
	siteID = strings.TrimSpace(siteID)
	if siteID == "" {
		return fmt.Errorf("solaredge oauth: empty site id")
	}
	c.mu.Lock()
	c.opts.SiteID = siteID
	c.token.SiteID = siteID
	path := c.opts.TokenPath
	tf := c.token
	c.mu.Unlock()
	if tf.RefreshToken == "" && tf.AccessToken == "" {
		return nil
	}
	return saveTokenFile(path, tf)
}
