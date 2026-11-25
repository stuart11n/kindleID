package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kindle-shazam/internal/solaredge"
)

type solarOAuthPending struct {
	state       string
	redirectURI string
	at          time.Time
}

// SetSolarOAuth attaches the SolarEdge OAuth client for settings-page auth.
func (d *Display) SetSolarOAuth(c *solaredge.OAuthClient) {
	d.mu.Lock()
	d.solarOAuth = c
	d.mu.Unlock()
}

func (d *Display) solarOAuthClient() *solaredge.OAuthClient {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.solarOAuth
}

func (d *Display) handleSolarEdge(w http.ResponseWriter, r *http.Request) {
	c := d.solarOAuthClient()
	switch r.URL.Path {
	case "/settings/solaredge.json":
		d.handleSolarEdgeJSON(w, r, c)
	case "/settings/solaredge/start":
		d.handleSolarEdgeStart(w, r, c)
	case "/settings/solaredge/callback":
		d.handleSolarEdgeCallback(w, r, c)
	default:
		http.NotFound(w, r)
	}
}

func (d *Display) handleSolarEdgeJSON(w http.ResponseWriter, r *http.Request, c *solaredge.OAuthClient) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		st := solaredge.AuthStatus{}
		if c != nil {
			st = c.Status()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"configured":     st.Configured,
			"authorized":     st.Authorized,
			"site_id":        st.SiteID,
			"redirect_uri":   solarRedirectURI(r),
			"authorize_hint": "Add the redirect_uri below to your SolarEdge developer app, then Connect.",
		})
	case http.MethodPost, http.MethodPut:
		if c == nil || !c.Configured() {
			http.Error(w, "SolarEdge client id/secret not configured on this server", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			SiteID      string `json:"site_id"`
			Code        string `json:"code"`
			RedirectURI string `json:"redirect_uri"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if site := strings.TrimSpace(body.SiteID); site != "" {
			if err := c.SetSiteID(site); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if code := strings.TrimSpace(body.Code); code != "" {
			redir := strings.TrimSpace(body.RedirectURI)
			if redir == "" {
				redir = solarRedirectURI(r)
			}
			if err := c.ExchangeCode(r.Context(), code, redir, body.SiteID); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		st := c.Status()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"configured": st.Configured,
			"authorized": st.Authorized,
			"site_id":    st.SiteID,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (d *Display) handleSolarEdgeStart(w http.ResponseWriter, r *http.Request, c *solaredge.OAuthClient) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c == nil || !c.Configured() {
		http.Error(w, "SolarEdge client id/secret not configured on this server", http.StatusServiceUnavailable)
		return
	}
	redir := solarRedirectURI(r)
	state, err := randomHex(16)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	authURL, err := c.AuthorizeURL(redir, state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	d.solarPendingMu.Lock()
	d.solarPending = solarOAuthPending{state: state, redirectURI: redir, at: time.Now()}
	d.solarPendingMu.Unlock()
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (d *Display) handleSolarEdgeCallback(w http.ResponseWriter, r *http.Request, c *solaredge.OAuthClient) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c == nil || !c.Configured() {
		http.Redirect(w, r, "/settings?solaredge=err&msg="+url.QueryEscape("client not configured"), http.StatusFound)
		return
	}
	q := r.URL.Query()
	if errMsg := q.Get("error"); errMsg != "" {
		desc := q.Get("error_description")
		msg := errMsg
		if desc != "" {
			msg = errMsg + ": " + desc
		}
		http.Redirect(w, r, "/settings?solaredge=err&msg="+url.QueryEscape(msg), http.StatusFound)
		return
	}
	code := strings.TrimSpace(q.Get("code"))
	if code == "" {
		http.Redirect(w, r, "/settings?solaredge=err&msg="+url.QueryEscape("missing code"), http.StatusFound)
		return
	}

	d.solarPendingMu.Lock()
	pending := d.solarPending
	d.solarPending = solarOAuthPending{}
	d.solarPendingMu.Unlock()

	if pending.state != "" && q.Get("state") != pending.state {
		http.Redirect(w, r, "/settings?solaredge=err&msg="+url.QueryEscape("state mismatch — try Connect again"), http.StatusFound)
		return
	}
	if !pending.at.IsZero() && time.Since(pending.at) > 15*time.Minute {
		http.Redirect(w, r, "/settings?solaredge=err&msg="+url.QueryEscape("auth timed out — try Connect again"), http.StatusFound)
		return
	}
	redir := pending.redirectURI
	if redir == "" {
		redir = solarRedirectURI(r)
	}
	site := strings.TrimSpace(q.Get("site_id"))
	if err := c.ExchangeCode(r.Context(), code, redir, site); err != nil {
		http.Redirect(w, r, "/settings?solaredge=err&msg="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/settings?solaredge=ok", http.StatusFound)
}

func solarRedirectURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.TrimSpace(strings.Split(proto, ",")[0])
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1:8080"
	}
	return scheme + "://" + host + "/settings/solaredge/callback"
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
