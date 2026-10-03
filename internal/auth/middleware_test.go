package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/auth"
)

type stubVerifier struct {
	principal auth.Principal
	err       error
}

func (s stubVerifier) Verify(context.Context, string) (auth.Principal, error) {
	if s.err != nil {
		return auth.Principal{}, s.err
	}
	return s.principal, nil
}

func TestAuthenticateMissingHeader(t *testing.T) {
	mw := auth.NewMiddleware(stubVerifier{}, auth.RoleWalletInternal)
	h := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	assertCode(t, rec, "unauthorized")
}

func TestAuthenticateInvalidToken(t *testing.T) {
	mw := auth.NewMiddleware(stubVerifier{err: errors.New("bad")}, auth.RoleWalletInternal)
	h := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestRequireInternalForbidden(t *testing.T) {
	mw := auth.NewMiddleware(stubVerifier{
		principal: auth.Principal{Subject: "provider-a", Roles: []string{}, ProviderID: "provider-a"},
	}, auth.RoleWalletInternal)

	h := mw.ProtectInternal(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer x")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	assertCode(t, rec, "forbidden")
}

func TestProtectInternalOK(t *testing.T) {
	mw := auth.NewMiddleware(stubVerifier{
		principal: auth.Principal{Subject: "svc", Roles: []string{auth.RoleWalletInternal}},
	}, auth.RoleWalletInternal)

	var sawSubject string
	h := mw.ProtectInternal(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("missing principal")
		}
		sawSubject = p.Subject
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rec.Code)
	}
	if sawSubject != "svc" {
		t.Fatalf("subject=%q", sawSubject)
	}
}

func TestRequireProvider(t *testing.T) {
	mw := auth.NewMiddleware(stubVerifier{
		principal: auth.Principal{Subject: "p", ProviderID: "provider-a", Roles: []string{}},
	}, auth.RoleWalletInternal)

	inner := mw.RequireProvider("provider-a", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h := mw.Authenticate(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}

	denied := mw.Authenticate(mw.RequireProvider("provider-b", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	rec2 := httptest.NewRecorder()
	denied.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec2.Code)
	}
}

func assertCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != want {
		t.Fatalf("code=%q want=%q", body["code"], want)
	}
}
