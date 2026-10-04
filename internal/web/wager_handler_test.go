package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/web"
)

type stubWager struct {
	processFn func(context.Context, usecasewager.ProcessInput) (usecasewager.ProcessResult, error)
	getFn     func(context.Context, string) (domainwager.Transaction, error)
	extFn     func(context.Context, string, string) (domainwager.Transaction, error)
}

func (s stubWager) Process(ctx context.Context, in usecasewager.ProcessInput) (usecasewager.ProcessResult, error) {
	return s.processFn(ctx, in)
}
func (s stubWager) Get(ctx context.Context, id string) (domainwager.Transaction, error) {
	return s.getFn(ctx, id)
}
func (s stubWager) GetByExternal(ctx context.Context, providerID, externalID string) (domainwager.Transaction, error) {
	return s.extFn(ctx, providerID, externalID)
}

func wagerRouter(t *testing.T, w stubWager, providerAuth bool) http.Handler {
	t.Helper()
	auth := web.NewProviderAuthForTest()
	if !providerAuth {
		auth = web.NewInternalAuthForTest()
	}
	return web.NewRouter(
		web.NewWalletHandlerForTest(stubWallet{}),
		web.NewWagerHandlerForTest(w),
		web.NewHealthHandlerForTest(),
		auth,
	)
}

func TestProcessWagerOK(t *testing.T) {
	bal := mustParse(t, "75.00", "BRL")
	tx := mustExternalProcessed(t, "provider-a", "ext-1", bal)
	mux := wagerRouter(t, stubWager{
		processFn: func(_ context.Context, in usecasewager.ProcessInput) (usecasewager.ProcessResult, error) {
			if in.IdempotencyKey != "prov-a:ext-1" || in.ProviderID != "provider-a" || in.PayloadHash == "" {
				t.Fatalf("input=%+v", in)
			}
			return usecasewager.ProcessResult{Transaction: tx, Balance: &bal}, nil
		},
	}, true)

	body := `{
		"providerId":"provider-a",
		"externalTransactionId":"ext-1",
		"playerId":"p-1",
		"walletId":"w-1",
		"roundId":"r-1",
		"gameId":"g-1",
		"kind":"BET",
		"money":{"amount":"25.00","currency":"BRL"}
	}`
	req := withAuth(httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewBufferString(body)))
	req.Header.Set("Idempotency-Key", "prov-a:ext-1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["status"] != "PROCESSED" || got["idempotentReplay"] != false {
		t.Fatalf("got=%v", got)
	}
}

func TestProcessMissingIdempotencyKey(t *testing.T) {
	mux := wagerRouter(t, stubWager{}, true)
	body := `{"providerId":"provider-a","externalTransactionId":"e","playerId":"p","walletId":"w","roundId":"r","gameId":"g","kind":"BET","money":{"amount":"1.00","currency":"BRL"}}`
	req := withAuth(httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewBufferString(body)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestProcessUnauthorized(t *testing.T) {
	mux := wagerRouter(t, stubWager{}, true)
	req := httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestProcessForbiddenInternal(t *testing.T) {
	mux := wagerRouter(t, stubWager{}, false)
	body := `{"providerId":"provider-a","externalTransactionId":"e","playerId":"p","walletId":"w","roundId":"r","gameId":"g","kind":"BET","money":{"amount":"1.00","currency":"BRL"}}`
	req := withAuth(httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewBufferString(body)))
	req.Header.Set("Idempotency-Key", "k")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProcessProviderMismatch(t *testing.T) {
	mux := wagerRouter(t, stubWager{}, true)
	body := `{"providerId":"provider-b","externalTransactionId":"e","playerId":"p","walletId":"w","roundId":"r","gameId":"g","kind":"BET","money":{"amount":"1.00","currency":"BRL"}}`
	req := withAuth(httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewBufferString(body)))
	req.Header.Set("Idempotency-Key", "k")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestProcessConflict(t *testing.T) {
	mux := wagerRouter(t, stubWager{
		processFn: func(context.Context, usecasewager.ProcessInput) (usecasewager.ProcessResult, error) {
			return usecasewager.ProcessResult{}, usecase.ErrConflict
		},
	}, true)
	body := `{"providerId":"provider-a","externalTransactionId":"e","playerId":"p","walletId":"w","roundId":"r","gameId":"g","kind":"BET","money":{"amount":"1.00","currency":"BRL"}}`
	req := withAuth(httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewBufferString(body)))
	req.Header.Set("Idempotency-Key", "k")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestGetWagerByID(t *testing.T) {
	bal := mustParse(t, "90.00", "BRL")
	own := mustExternalProcessed(t, "provider-a", "ext-own", bal)
	other := mustExternalProcessed(t, "provider-b", "ext-other", bal)

	mux := wagerRouter(t, stubWager{
		getFn: func(_ context.Context, id string) (domainwager.Transaction, error) {
			switch id {
			case "own":
				return own, nil
			case "other":
				return other, nil
			default:
				return domainwager.Transaction{}, usecase.ErrNotFound
			}
		},
	}, true)

	okReq := withAuth(httptest.NewRequest(http.MethodGet, "/wagering/transactions/own", nil))
	okRec := httptest.NewRecorder()
	mux.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("own status=%d", okRec.Code)
	}

	forbidReq := withAuth(httptest.NewRequest(http.MethodGet, "/wagering/transactions/other", nil))
	forbidRec := httptest.NewRecorder()
	mux.ServeHTTP(forbidRec, forbidReq)
	if forbidRec.Code != http.StatusForbidden {
		t.Fatalf("other status=%d", forbidRec.Code)
	}

	missReq := withAuth(httptest.NewRequest(http.MethodGet, "/wagering/transactions/missing", nil))
	missRec := httptest.NewRecorder()
	mux.ServeHTTP(missRec, missReq)
	if missRec.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", missRec.Code)
	}
}

func TestGetByExternalProviderPath(t *testing.T) {
	bal := mustParse(t, "90.00", "BRL")
	tx := mustExternalProcessed(t, "provider-a", "ext-1", bal)
	mux := wagerRouter(t, stubWager{
		extFn: func(_ context.Context, providerID, externalID string) (domainwager.Transaction, error) {
			if providerID != "provider-a" || externalID != "ext-1" {
				t.Fatalf("args=%s %s", providerID, externalID)
			}
			return tx, nil
		},
	}, true)

	okReq := withAuth(httptest.NewRequest(http.MethodGet, "/providers/provider-a/wagering/transactions/ext-1", nil))
	okRec := httptest.NewRecorder()
	mux.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", okRec.Code, okRec.Body.String())
	}

	badReq := withAuth(httptest.NewRequest(http.MethodGet, "/providers/provider-b/wagering/transactions/ext-1", nil))
	badRec := httptest.NewRecorder()
	mux.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusForbidden {
		t.Fatalf("mismatch status=%d", badRec.Code)
	}
}

func mustExternalProcessed(t *testing.T, providerID, externalID string, bal money.Money) domainwager.Transaction {
	t.Helper()
	now := time.Date(2026, 10, 4, 16, 0, 0, 0, time.UTC)
	amount := mustParse(t, "10.00", "BRL")
	tx, err := domainwager.FromPersisted(domainwager.PersistedParams{
		ID: "tx-" + externalID, Origin: domainwager.OriginExternal, Kind: domainwager.KindBet,
		Status: domainwager.StatusProcessed, WalletID: "w-1", PlayerID: "p-1",
		Amount: amount, CreatedAt: now, UpdatedAt: now,
		ProviderID: providerID, ExternalTransactionID: externalID,
		IdempotencyKey: providerID + ":" + externalID, PayloadHash: "hash",
		RoundID: "r-1", GameID: "g-1", ResultBalance: &bal,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tx
}
