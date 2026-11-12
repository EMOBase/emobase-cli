// Package keycloak is a minimal Keycloak admin API client: it checks that a
// realm is reachable, looks up a client and manages redirect URIs.
package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a Keycloak server's public and admin endpoints.
type Client struct {
	Base     string
	User     string
	Password string
	HTTP     *http.Client
}

// New builds a client. base is KEYCLOAK_PUBLIC_URL, e.g.
// http://localhost:8081/keycloak.
func New(base, user, password string) *Client {
	return &Client{
		Base:     strings.TrimRight(base, "/"),
		User:     user,
		Password: password,
		HTTP:     &http.Client{Timeout: 15 * time.Second},
	}
}

// RealmReachable checks the public OIDC discovery document, which needs no
// admin credentials.
func (c *Client) RealmReachable(ctx context.Context, realm string) (bool, error) {
	u := fmt.Sprintf("%s/realms/%s/.well-known/openid-configuration", c.Base, url.PathEscape(realm))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("GET %s: %s", u, resp.Status)
}

func (c *Client) token(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", "admin-cli")
	form.Set("username", c.User)
	form.Set("password", c.Password)
	u := c.Base + "/realms/master/protocol/openid-connect/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("admin token: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("admin token: empty access_token")
	}
	return out.AccessToken, nil
}

func (c *Client) adminDo(ctx context.Context, method, path string, body any, out any) (int, error) {
	token, err := c.token(ctx)
	if err != nil {
		return 0, err
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, err
		}
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	return resp.StatusCode, nil
}

// ClientRep is the Keycloak client representation we care about.
type ClientRep struct {
	ID           string   `json:"id"`
	ClientID     string   `json:"clientId"`
	RedirectURIs []string `json:"redirectUris"`
}

// ClientByID finds a client by its clientId.
func (c *Client) ClientByID(ctx context.Context, realm, clientID string) (*ClientRep, error) {
	var reps []ClientRep
	path := fmt.Sprintf("/admin/realms/%s/clients?clientId=%s", url.PathEscape(realm), url.QueryEscape(clientID))
	if _, err := c.adminDo(ctx, http.MethodGet, path, nil, &reps); err != nil {
		return nil, err
	}
	if len(reps) == 0 {
		return nil, nil
	}
	return &reps[0], nil
}

// AddRedirectURI appends origin to the client's redirect URIs when no existing
// pattern already matches it. It is idempotent.
func (c *Client) AddRedirectURI(ctx context.Context, realm, clientID, origin string) (bool, error) {
	rep, err := c.ClientByID(ctx, realm, clientID)
	if err != nil {
		return false, err
	}
	if rep == nil {
		return false, fmt.Errorf("client %q not found in realm %q", clientID, realm)
	}
	if MatchesOrigin(rep.RedirectURIs, origin) {
		return false, nil
	}
	// Fetch the full representation, then update, so no fields are dropped.
	var full map[string]any
	path := fmt.Sprintf("/admin/realms/%s/clients/%s", url.PathEscape(realm), url.PathEscape(rep.ID))
	if _, err := c.adminDo(ctx, http.MethodGet, path, nil, &full); err != nil {
		return false, err
	}
	uris, _ := full["redirectUris"].([]any)
	full["redirectUris"] = append(uris, origin)
	if _, err := c.adminDo(ctx, http.MethodPut, path, full, nil); err != nil {
		return false, err
	}
	return true, nil
}

// MatchesOrigin reports whether any Keycloak redirect pattern covers origin.
// Keycloak supports an exact value, a trailing /* wildcard and a global *.
func MatchesOrigin(patterns []string, origin string) bool {
	origin = strings.TrimRight(origin, "/")
	for _, p := range patterns {
		p = strings.TrimRight(strings.TrimSpace(p), "/")
		switch {
		case p == "*" || p == origin:
			return true
		case strings.HasSuffix(p, "/*"):
			prefix := strings.TrimSuffix(p, "/*")
			if origin == prefix || strings.HasPrefix(origin, prefix+"/") {
				return true
			}
		case p == origin+"/*":
			return true
		}
	}
	return false
}
