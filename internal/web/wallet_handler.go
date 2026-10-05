package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/observability"
	usecasewallet "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wallet"
)

// walletService is the application port used by HTTP handlers (testable).
type walletService interface {
	Open(ctx context.Context, in usecasewallet.OpenInput) (usecasewallet.WalletView, error)
	Get(ctx context.Context, walletID string) (usecasewallet.WalletView, error)
	ListLedger(ctx context.Context, in usecasewallet.ListLedgerInput) (usecasewallet.ListLedgerOutput, error)
	Reconcile(ctx context.Context, walletID string) (usecasewallet.ReconcileOutput, error)
}

// WalletHandler exposes wallet HTTP routes.
type WalletHandler struct {
	uc walletService
}

func NewWalletHandler(uc *usecasewallet.UseCase) *WalletHandler {
	return &WalletHandler{uc: uc}
}

type moneyDTO struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type openWalletRequest struct {
	PlayerID       string   `json:"playerId"`
	InitialBalance moneyDTO `json:"initialBalance"`
}

type walletResponse struct {
	ID       string   `json:"id"`
	PlayerID string   `json:"playerId"`
	Balance  moneyDTO `json:"balance"`
	Version  int64    `json:"version"`
}

type ledgerEntryResponse struct {
	ID            string    `json:"id"`
	WalletID      string    `json:"walletId"`
	TransactionID string    `json:"transactionId"`
	Direction     string    `json:"direction"`
	Amount        moneyDTO  `json:"amount"`
	BalanceBefore moneyDTO  `json:"balanceBefore"`
	BalanceAfter  moneyDTO  `json:"balanceAfter"`
	CreatedAt     time.Time `json:"createdAt"`
}

type listLedgerResponse struct {
	Entries    []ledgerEntryResponse `json:"entries"`
	NextCursor string                `json:"nextCursor"`
}

type reconcileResponse struct {
	WalletID          string   `json:"walletId"`
	StoredBalance     moneyDTO `json:"storedBalance"`
	CalculatedBalance moneyDTO `json:"calculatedBalance"`
	Difference        moneyDTO `json:"difference"`
	Consistent        bool     `json:"consistent"`
	CheckedEntries    int      `json:"checkedEntries"`
}

func (h *WalletHandler) Open(w http.ResponseWriter, r *http.Request) {
	var req openWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "invalid json body")
		return
	}
	bal, err := money.Parse(req.InitialBalance.Currency, req.InitialBalance.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "invalid initialBalance")
		return
	}

	out, err := h.uc.Open(r.Context(), usecasewallet.OpenInput{
		PlayerID:       req.PlayerID,
		InitialBalance: bal,
	})
	if err != nil {
		mapUseCaseError(w, err)
		return
	}
	slog.Info("wallet opened",
		"correlationId", observability.CorrelationID(r.Context()),
		"walletId", out.ID,
		"playerId", out.PlayerID,
	)
	writeJSON(w, http.StatusCreated, walletViewResponse(out))
}

func (h *WalletHandler) Get(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")
	out, err := h.uc.Get(r.Context(), walletID)
	if err != nil {
		mapUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, walletViewResponse(out))
}

func (h *WalletHandler) ListLedger(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_input", "invalid limit")
			return
		}
		limit = n
	}

	out, err := h.uc.ListLedger(r.Context(), usecasewallet.ListLedgerInput{
		WalletID: walletID,
		Cursor:   r.URL.Query().Get("cursor"),
		Limit:    limit,
	})
	if err != nil {
		mapUseCaseError(w, err)
		return
	}

	entries := make([]ledgerEntryResponse, 0, len(out.Entries))
	for _, e := range out.Entries {
		entries = append(entries, ledgerEntryResponse{
			ID:            e.ID,
			WalletID:      e.WalletID,
			TransactionID: e.TransactionID,
			Direction:     string(e.Direction),
			Amount:        moneyResponse(e.Amount),
			BalanceBefore: moneyResponse(e.BalanceBefore),
			BalanceAfter:  moneyResponse(e.BalanceAfter),
			CreatedAt:     e.CreatedAt.UTC(),
		})
	}
	writeJSON(w, http.StatusOK, listLedgerResponse{
		Entries:    entries,
		NextCursor: out.NextCursor,
	})
}

func (h *WalletHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("walletId")
	out, err := h.uc.Reconcile(r.Context(), walletID)
	if err != nil {
		mapUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reconcileResponse{
		WalletID:          out.WalletID,
		StoredBalance:     moneyResponse(out.StoredBalance),
		CalculatedBalance: moneyResponse(out.CalculatedBalance),
		Difference:        moneyResponse(out.Difference),
		Consistent:        out.Consistent,
		CheckedEntries:    out.CheckedEntries,
	})
}

func walletViewResponse(v usecasewallet.WalletView) walletResponse {
	return walletResponse{
		ID:       v.ID,
		PlayerID: v.PlayerID,
		Balance:  moneyResponse(v.Balance),
		Version:  v.Version,
	}
}

func moneyResponse(m money.Money) moneyDTO {
	return moneyDTO{Amount: m.AmountString(), Currency: m.Currency()}
}
