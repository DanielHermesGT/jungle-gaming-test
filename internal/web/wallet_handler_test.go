package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwallet "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
	usecasewallet "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/web"
)

type stubWallet struct {
	openFn      func(context.Context, usecasewallet.OpenInput) (usecasewallet.WalletView, error)
	getFn       func(context.Context, string) (usecasewallet.WalletView, error)
	listFn      func(context.Context, usecasewallet.ListLedgerInput) (usecasewallet.ListLedgerOutput, error)
	reconcileFn func(context.Context, string) (usecasewallet.ReconcileOutput, error)
}

func (s stubWallet) Open(ctx context.Context, in usecasewallet.OpenInput) (usecasewallet.WalletView, error) {
	return s.openFn(ctx, in)
}
func (s stubWallet) Get(ctx context.Context, id string) (usecasewallet.WalletView, error) {
	return s.getFn(ctx, id)
}
func (s stubWallet) ListLedger(ctx context.Context, in usecasewallet.ListLedgerInput) (usecasewallet.ListLedgerOutput, error) {
	return s.listFn(ctx, in)
}
func (s stubWallet) Reconcile(ctx context.Context, id string) (usecasewallet.ReconcileOutput, error) {
	return s.reconcileFn(ctx, id)
}

func withAuth(req *http.Request) *http.Request {
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}

func TestOpenCreated(t *testing.T) {
	bal := mustParse(t, "100.00", "BRL")
	h := web.NewWalletHandlerForTest(stubWallet{
		openFn: func(_ context.Context, in usecasewallet.OpenInput) (usecasewallet.WalletView, error) {
			if in.PlayerID != "player-1" || !in.InitialBalance.Equal(bal) {
				t.Fatalf("input=%+v", in)
			}
			return usecasewallet.WalletView{
				ID: "w1", PlayerID: in.PlayerID, Balance: bal, Version: 1,
			}, nil
		},
	})
	mux := web.NewRouter(h, web.NewHealthHandlerForTest(), web.NewInternalAuthForTest())

	body := `{"playerId":"player-1","initialBalance":{"amount":"100.00","currency":"BRL"}}`
	req := withAuth(httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewBufferString(body)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "w1" || got["version"].(float64) != 1 {
		t.Fatalf("got=%v", got)
	}
}

func TestOpenUnauthorized(t *testing.T) {
	h := web.NewWalletHandlerForTest(stubWallet{})
	mux := web.NewRouter(h, web.NewHealthHandlerForTest(), web.NewInternalAuthForTest())

	body := `{"playerId":"p","initialBalance":{"amount":"0.00","currency":"BRL"}}`
	req := httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestOpenForbiddenForProvider(t *testing.T) {
	h := web.NewWalletHandlerForTest(stubWallet{})
	mux := web.NewRouter(h, web.NewHealthHandlerForTest(), web.NewProviderAuthForTest())

	body := `{"playerId":"p","initialBalance":{"amount":"0.00","currency":"BRL"}}`
	req := withAuth(httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewBufferString(body)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetNotFound(t *testing.T) {
	h := web.NewWalletHandlerForTest(stubWallet{
		getFn: func(context.Context, string) (usecasewallet.WalletView, error) {
			return usecasewallet.WalletView{}, usecase.ErrNotFound
		},
	})
	mux := web.NewRouter(h, web.NewHealthHandlerForTest(), web.NewInternalAuthForTest())

	req := withAuth(httptest.NewRequest(http.MethodGet, "/wallets/missing", nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestOpenConflict(t *testing.T) {
	h := web.NewWalletHandlerForTest(stubWallet{
		openFn: func(context.Context, usecasewallet.OpenInput) (usecasewallet.WalletView, error) {
			return usecasewallet.WalletView{}, usecase.ErrConflict
		},
	})
	mux := web.NewRouter(h, web.NewHealthHandlerForTest(), web.NewInternalAuthForTest())

	body := `{"playerId":"dup","initialBalance":{"amount":"0.00","currency":"BRL"}}`
	req := withAuth(httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewBufferString(body)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListLedgerQuery(t *testing.T) {
	h := web.NewWalletHandlerForTest(stubWallet{
		listFn: func(_ context.Context, in usecasewallet.ListLedgerInput) (usecasewallet.ListLedgerOutput, error) {
			if in.WalletID != "w1" || in.Cursor != "cur" || in.Limit != 2 {
				t.Fatalf("input=%+v", in)
			}
			return usecasewallet.ListLedgerOutput{
				Entries: []usecasewallet.LedgerEntryView{{
					ID: "e1", WalletID: "w1", TransactionID: "t1",
					Direction:     domainwallet.DirectionCredit,
					Amount:        mustParse(t, "10.00", "BRL"),
					BalanceBefore: mustParse(t, "0.00", "BRL"),
					BalanceAfter:  mustParse(t, "10.00", "BRL"),
					CreatedAt:     time.Unix(1, 0).UTC(),
				}},
				NextCursor: "next",
			}, nil
		},
	})
	mux := web.NewRouter(h, web.NewHealthHandlerForTest(), web.NewInternalAuthForTest())

	req := withAuth(httptest.NewRequest(http.MethodGet, "/wallets/w1/ledger?cursor=cur&limit=2", nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["nextCursor"] != "next" {
		t.Fatalf("got=%v", got)
	}
}

func TestReconcileOK(t *testing.T) {
	bal := mustParse(t, "50.00", "BRL")
	zero := mustParse(t, "0.00", "BRL")
	h := web.NewWalletHandlerForTest(stubWallet{
		reconcileFn: func(_ context.Context, id string) (usecasewallet.ReconcileOutput, error) {
			if id != "w1" {
				return usecasewallet.ReconcileOutput{}, errors.New("bad id")
			}
			return usecasewallet.ReconcileOutput{
				WalletID: "w1", StoredBalance: bal, CalculatedBalance: bal,
				Difference: zero, Consistent: true, CheckedEntries: 1,
			}, nil
		},
	})
	mux := web.NewRouter(h, web.NewHealthHandlerForTest(), web.NewInternalAuthForTest())

	req := withAuth(httptest.NewRequest(http.MethodPost, "/wallets/w1/reconciliation", nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestHealthLivePublic(t *testing.T) {
	mux := web.NewRouter(web.NewWalletHandlerForTest(stubWallet{}), web.NewHealthHandlerForTest(), web.NewInternalAuthForTest())
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}

func mustParse(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.Parse(currency, amount)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
