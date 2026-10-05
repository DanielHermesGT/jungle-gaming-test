package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestKeycloakRejectsUnauthorized skips unless Keycloak is reachable.
// Proves missing/invalid tokens get 401 from a live API (or documents IdP token endpoint).
func TestKeycloakTokenAndAudience(t *testing.T) {
	issuer := os.Getenv("OIDC_ISSUER_URL")
	if issuer == "" {
		t.Skip("OIDC_ISSUER_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wellKnown := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("keycloak unreachable: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("discovery status=%d", res.StatusCode)
	}

	tokenURL := strings.TrimRight(issuer, "/") + "/protocol/openid-connect/token"
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"jungle-internal"},
		"client_secret": {"jungle-internal-secret"},
	}
	tres, err := http.PostForm(tokenURL, form)
	if err != nil {
		t.Fatal(err)
	}
	defer tres.Body.Close()
	body, _ := io.ReadAll(tres.Body)
	if tres.StatusCode != http.StatusOK {
		t.Fatalf("token status=%d body=%s", tres.StatusCode, body)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		t.Fatalf("token parse: %v body=%s", err, body)
	}

	api := os.Getenv("API_BASE_URL")
	if api == "" {
		api = "http://localhost:8080"
	}
	// Unauthorized: no bearer
	ureq, _ := http.NewRequestWithContext(ctx, http.MethodPost, api+"/wallets", strings.NewReader(`{}`))
	ureq.Header.Set("Content-Type", "application/json")
	ures, err := http.DefaultClient.Do(ureq)
	if err != nil {
		t.Skipf("api unreachable: %v", err)
	}
	defer ures.Body.Close()
	if ures.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d", ures.StatusCode)
	}

	// Provider token must not open wallets (403) — no financial side effect.
	pform := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"provider-a"},
		"client_secret": {"provider-a-secret"},
	}
	pres, err := http.PostForm(tokenURL, pform)
	if err != nil {
		t.Fatal(err)
	}
	defer pres.Body.Close()
	pbody, _ := io.ReadAll(pres.Body)
	var ptok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(pbody, &ptok)
	if ptok.AccessToken == "" {
		t.Fatalf("provider token empty: %s", pbody)
	}
	creq, _ := http.NewRequestWithContext(ctx, http.MethodPost, api+"/wallets",
		strings.NewReader(`{"playerId":"nope","initialBalance":{"amount":"1.00","currency":"BRL"}}`))
	creq.Header.Set("Authorization", "Bearer "+ptok.AccessToken)
	creq.Header.Set("Content-Type", "application/json")
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	defer cres.Body.Close()
	if cres.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 for provider on /wallets, got %d", cres.StatusCode)
	}
}
