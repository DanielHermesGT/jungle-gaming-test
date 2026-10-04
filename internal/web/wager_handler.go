package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/auth"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
)

// wagerService is the application port used by HTTP handlers (testable).
type wagerService interface {
	Process(ctx context.Context, in usecasewager.ProcessInput) (usecasewager.ProcessResult, error)
	Get(ctx context.Context, id string) (domainwager.Transaction, error)
	GetByExternal(ctx context.Context, providerID, externalID string) (domainwager.Transaction, error)
}

// WagerHandler exposes provider wagering HTTP routes.
type WagerHandler struct {
	uc wagerService
}

func NewWagerHandler(uc *usecasewager.UseCase) *WagerHandler {
	return &WagerHandler{uc: uc}
}

type processWagerRequest struct {
	ProviderID                     string   `json:"providerId"`
	ExternalTransactionID          string   `json:"externalTransactionId"`
	PlayerID                       string   `json:"playerId"`
	WalletID                       string   `json:"walletId"`
	RoundID                        string   `json:"roundId"`
	GameID                         string   `json:"gameId"`
	Kind                           string   `json:"kind"`
	Money                          moneyDTO `json:"money"`
	ReferenceExternalTransactionID string   `json:"referenceExternalTransactionId"`
}

type processWagerResponse struct {
	TransactionID    string    `json:"transactionId"`
	Status           string    `json:"status"`
	Balance          *moneyDTO `json:"balance,omitempty"`
	FailureCode      string    `json:"failureCode,omitempty"`
	IdempotentReplay bool      `json:"idempotentReplay"`
}

type wagerTransactionResponse struct {
	TransactionID                  string     `json:"transactionId"`
	ProviderID                     string     `json:"providerId,omitempty"`
	ExternalTransactionID          string     `json:"externalTransactionId,omitempty"`
	PlayerID                       string     `json:"playerId"`
	WalletID                       string     `json:"walletId"`
	RoundID                        string     `json:"roundId,omitempty"`
	GameID                         string     `json:"gameId,omitempty"`
	Kind                           string     `json:"kind"`
	Money                          moneyDTO   `json:"money"`
	ReferenceExternalTransactionID string     `json:"referenceExternalTransactionId,omitempty"`
	Status                         string     `json:"status"`
	FailureCode                    string     `json:"failureCode,omitempty"`
	Balance                        *moneyDTO  `json:"balance,omitempty"`
	PendingReferenceUntil          *time.Time `json:"pendingReferenceUntil,omitempty"`
	CreatedAt                      time.Time  `json:"createdAt"`
	UpdatedAt                      time.Time  `json:"updatedAt"`
}

func (h *WagerHandler) Process(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ProviderID == "" {
		writeError(w, http.StatusForbidden, "forbidden", "provider not authorized")
		return
	}

	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "Idempotency-Key header is required")
		return
	}

	var req processWagerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "invalid json body")
		return
	}
	if req.ProviderID != p.ProviderID {
		writeError(w, http.StatusForbidden, "forbidden", "provider not authorized")
		return
	}

	kind, err := domainwager.ParseKind(req.Kind)
	if err != nil || !kind.IsExternal() {
		writeError(w, http.StatusBadRequest, "invalid_input", "invalid kind")
		return
	}
	amount, err := money.Parse(req.Money.Currency, req.Money.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "invalid money")
		return
	}

	hash, err := usecasewager.CanonicalPayloadHash(usecasewager.PayloadFields{
		ProviderID:                     req.ProviderID,
		ExternalTransactionID:          req.ExternalTransactionID,
		PlayerID:                       req.PlayerID,
		WalletID:                       req.WalletID,
		RoundID:                        req.RoundID,
		GameID:                         req.GameID,
		Kind:                           kind,
		Amount:                         amount,
		ReferenceExternalTransactionID: req.ReferenceExternalTransactionID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "invalid payload")
		return
	}

	out, err := h.uc.Process(r.Context(), usecasewager.ProcessInput{
		ProviderID:                     req.ProviderID,
		ExternalTransactionID:          req.ExternalTransactionID,
		IdempotencyKey:                 idemKey,
		PlayerID:                       req.PlayerID,
		WalletID:                       req.WalletID,
		RoundID:                        req.RoundID,
		GameID:                         req.GameID,
		Kind:                           kind,
		Amount:                         amount,
		ReferenceExternalTransactionID: req.ReferenceExternalTransactionID,
		PayloadHash:                    hash,
	})
	if err != nil {
		mapUseCaseError(w, err)
		return
	}

	resp := processWagerResponse{
		TransactionID:    out.Transaction.ID(),
		Status:           string(out.Transaction.Status()),
		FailureCode:      string(out.Transaction.FailureCode()),
		IdempotentReplay: out.IdempotentReplay,
	}
	if out.Balance != nil {
		m := moneyResponse(*out.Balance)
		resp.Balance = &m
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *WagerHandler) Get(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ProviderID == "" {
		writeError(w, http.StatusForbidden, "forbidden", "provider not authorized")
		return
	}

	id := r.PathValue("transactionId")
	tx, err := h.uc.Get(r.Context(), id)
	if err != nil {
		mapUseCaseError(w, err)
		return
	}
	if tx.ProviderID() != p.ProviderID {
		writeError(w, http.StatusForbidden, "forbidden", "provider not authorized")
		return
	}
	writeJSON(w, http.StatusOK, wagerViewResponse(tx))
}

func (h *WagerHandler) GetByExternal(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerId")
	externalID := r.PathValue("externalTransactionId")
	tx, err := h.uc.GetByExternal(r.Context(), providerID, externalID)
	if err != nil {
		mapUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wagerViewResponse(tx))
}

func wagerViewResponse(tx domainwager.Transaction) wagerTransactionResponse {
	resp := wagerTransactionResponse{
		TransactionID:                  tx.ID(),
		ProviderID:                     tx.ProviderID(),
		ExternalTransactionID:          tx.ExternalTransactionID(),
		PlayerID:                       tx.PlayerID(),
		WalletID:                       tx.WalletID(),
		RoundID:                        tx.RoundID(),
		GameID:                         tx.GameID(),
		Kind:                           string(tx.Kind()),
		Money:                          moneyResponse(tx.Amount()),
		ReferenceExternalTransactionID: tx.ReferenceExternalTransaction(),
		Status:                         string(tx.Status()),
		FailureCode:                    string(tx.FailureCode()),
		CreatedAt:                      tx.CreatedAt().UTC(),
		UpdatedAt:                      tx.UpdatedAt().UTC(),
	}
	if bal := tx.ResultBalance(); bal != nil {
		m := moneyResponse(*bal)
		resp.Balance = &m
	}
	if until := tx.PendingReferenceUntil(); !until.IsZero() {
		u := until.UTC()
		resp.PendingReferenceUntil = &u
	}
	return resp
}
