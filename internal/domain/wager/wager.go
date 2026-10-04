package wager

import (
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
)

// Transaction is the wager aggregate (README WagerTransaction).
type Transaction struct {
	id        string
	origin    Origin
	kind      Kind
	status    Status
	walletID  string
	playerID  string
	amount    money.Money
	createdAt time.Time
	updatedAt time.Time

	failureCode   FailureCode
	resultBalance *money.Money

	// EXTERNAL-only fields (empty for INTERNAL).
	providerID                   string
	externalTransactionID        string
	idempotencyKey               string
	payloadHash                  string
	roundID                      string
	gameID                       string
	referenceExternalTransaction string
	resolvedReferenceID          string
}

// NewOpening creates an INTERNAL OPENING already in PROCESSED (README §9).
func NewOpening(p OpeningParams) (Transaction, error) {
	if p.ID == "" || p.WalletID == "" || p.PlayerID == "" {
		return Transaction{}, ErrInvalidInput
	}
	if !moneyInitialized(p.Amount) || p.Amount.IsNegative() {
		return Transaction{}, ErrInvalidAmount
	}
	return Transaction{
		id:        p.ID,
		origin:    OriginInternal,
		kind:      KindOpening,
		status:    StatusProcessed,
		walletID:  p.WalletID,
		playerID:  p.PlayerID,
		amount:    p.Amount,
		createdAt: p.Now,
		updatedAt: p.Now,
	}, nil
}

// NewExternal creates a provider-facing transaction in PENDING.
func NewExternal(p ExternalParams) (Transaction, error) {
	if p.ID == "" || p.WalletID == "" || p.PlayerID == "" {
		return Transaction{}, ErrInvalidInput
	}
	if p.ProviderID == "" || p.ExternalTransactionID == "" || p.IdempotencyKey == "" ||
		p.PayloadHash == "" || p.RoundID == "" || p.GameID == "" {
		return Transaction{}, ErrInvalidInput
	}
	if p.Kind == KindOpening || !p.Kind.IsExternal() {
		return Transaction{}, ErrInvalidKind
	}
	if err := validateAmountForKind(p.Kind, p.Amount); err != nil {
		return Transaction{}, err
	}
	if p.Kind.RequiresReference() && p.ReferenceExternalTransaction == "" {
		return Transaction{}, ErrMissingReference
	}

	return Transaction{
		id:                           p.ID,
		origin:                       OriginExternal,
		kind:                         p.Kind,
		status:                       StatusPending,
		walletID:                     p.WalletID,
		playerID:                     p.PlayerID,
		amount:                       p.Amount,
		createdAt:                    p.Now,
		updatedAt:                    p.Now,
		providerID:                   p.ProviderID,
		externalTransactionID:        p.ExternalTransactionID,
		idempotencyKey:               p.IdempotencyKey,
		payloadHash:                  p.PayloadHash,
		roundID:                      p.RoundID,
		gameID:                       p.GameID,
		referenceExternalTransaction: p.ReferenceExternalTransaction,
	}, nil
}

// PersistedParams rebuilds a Transaction from storage without applying transitions.
type PersistedParams struct {
	ID                           string
	Origin                       Origin
	Kind                         Kind
	Status                       Status
	WalletID                     string
	PlayerID                     string
	Amount                       money.Money
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
	FailureCode                  FailureCode
	ResultBalance                *money.Money
	ProviderID                   string
	ExternalTransactionID        string
	IdempotencyKey               string
	PayloadHash                  string
	RoundID                      string
	GameID                       string
	ReferenceExternalTransaction string
	ResolvedReferenceID          string
}

// FromPersisted rebuilds a Transaction from persisted state.
func FromPersisted(p PersistedParams) (Transaction, error) {
	if p.ID == "" || p.WalletID == "" || p.PlayerID == "" {
		return Transaction{}, ErrInvalidInput
	}
	if _, err := ParseOrigin(string(p.Origin)); err != nil {
		return Transaction{}, err
	}
	if _, err := ParseKind(string(p.Kind)); err != nil {
		return Transaction{}, err
	}
	if _, err := ParseStatus(string(p.Status)); err != nil {
		return Transaction{}, err
	}
	if !moneyInitialized(p.Amount) {
		return Transaction{}, ErrInvalidAmount
	}

	switch p.Origin {
	case OriginInternal:
		if p.Kind != KindOpening {
			return Transaction{}, ErrInvalidKind
		}
		if p.ProviderID != "" || p.ExternalTransactionID != "" || p.IdempotencyKey != "" {
			return Transaction{}, ErrInvalidInput
		}
	case OriginExternal:
		if !p.Kind.IsExternal() {
			return Transaction{}, ErrInvalidKind
		}
		if p.ProviderID == "" || p.ExternalTransactionID == "" || p.IdempotencyKey == "" ||
			p.PayloadHash == "" || p.RoundID == "" || p.GameID == "" {
			return Transaction{}, ErrInvalidInput
		}
	}

	if p.FailureCode != "" && !p.FailureCode.Valid() {
		return Transaction{}, ErrInvalidInput
	}
	if p.ResultBalance != nil && !moneyInitialized(*p.ResultBalance) {
		return Transaction{}, ErrInvalidAmount
	}

	return Transaction{
		id:                           p.ID,
		origin:                       p.Origin,
		kind:                         p.Kind,
		status:                       p.Status,
		walletID:                     p.WalletID,
		playerID:                     p.PlayerID,
		amount:                       p.Amount,
		createdAt:                    p.CreatedAt,
		updatedAt:                    p.UpdatedAt,
		failureCode:                  p.FailureCode,
		resultBalance:                p.ResultBalance,
		providerID:                   p.ProviderID,
		externalTransactionID:        p.ExternalTransactionID,
		idempotencyKey:               p.IdempotencyKey,
		payloadHash:                  p.PayloadHash,
		roundID:                      p.RoundID,
		gameID:                       p.GameID,
		referenceExternalTransaction: p.ReferenceExternalTransaction,
		resolvedReferenceID:          p.ResolvedReferenceID,
	}, nil
}

func (t Transaction) ID() string                           { return t.id }
func (t Transaction) Origin() Origin                       { return t.origin }
func (t Transaction) Kind() Kind                           { return t.kind }
func (t Transaction) Status() Status                       { return t.status }
func (t Transaction) WalletID() string                     { return t.walletID }
func (t Transaction) PlayerID() string                     { return t.playerID }
func (t Transaction) Amount() money.Money                  { return t.amount }
func (t Transaction) CreatedAt() time.Time                 { return t.createdAt }
func (t Transaction) UpdatedAt() time.Time                 { return t.updatedAt }
func (t Transaction) FailureCode() FailureCode             { return t.failureCode }
func (t Transaction) ResultBalance() *money.Money          { return t.resultBalance }
func (t Transaction) ProviderID() string                   { return t.providerID }
func (t Transaction) ExternalTransactionID() string        { return t.externalTransactionID }
func (t Transaction) IdempotencyKey() string               { return t.idempotencyKey }
func (t Transaction) PayloadHash() string                  { return t.payloadHash }
func (t Transaction) RoundID() string                      { return t.roundID }
func (t Transaction) GameID() string                       { return t.gameID }
func (t Transaction) ReferenceExternalTransaction() string { return t.referenceExternalTransaction }
func (t Transaction) ResolvedReferenceID() string          { return t.resolvedReferenceID }

func (t Transaction) valid() bool {
	return t.id != "" && t.walletID != "" && t.playerID != "" && moneyInitialized(t.amount)
}

// AwaitReference moves PENDING → PENDING_REFERENCE.
func (t Transaction) AwaitReference(now time.Time) (Transaction, error) {
	if !t.valid() {
		return Transaction{}, ErrUninitialized
	}
	if t.status.IsTerminal() {
		return Transaction{}, ErrTerminalStatus
	}
	if t.status != StatusPending {
		return Transaction{}, ErrInvalidTransition
	}
	next := t
	next.status = StatusPendingReference
	next.updatedAt = now
	return next, nil
}

// MarkProcessed moves PENDING or PENDING_REFERENCE → PROCESSED.
// resultBalance is optional (e.g. LOSS may pass nil).
func (t Transaction) MarkProcessed(resultBalance *money.Money, now time.Time) (Transaction, error) {
	if !t.valid() {
		return Transaction{}, ErrUninitialized
	}
	if t.status.IsTerminal() {
		return Transaction{}, ErrTerminalStatus
	}
	if t.status != StatusPending && t.status != StatusPendingReference {
		return Transaction{}, ErrInvalidTransition
	}
	if resultBalance != nil && !moneyInitialized(*resultBalance) {
		return Transaction{}, ErrInvalidAmount
	}
	next := t
	next.status = StatusProcessed
	next.resultBalance = resultBalance
	next.failureCode = ""
	next.updatedAt = now
	return next, nil
}

// MarkRejected moves PENDING or PENDING_REFERENCE → REJECTED with a business failure code.
func (t Transaction) MarkRejected(code FailureCode, now time.Time) (Transaction, error) {
	return t.markTerminal(StatusRejected, code, now)
}

// MarkFailed moves PENDING or PENDING_REFERENCE → FAILED (permanent infra/business failure).
func (t Transaction) MarkFailed(code FailureCode, now time.Time) (Transaction, error) {
	return t.markTerminal(StatusFailed, code, now)
}

func (t Transaction) markTerminal(target Status, code FailureCode, now time.Time) (Transaction, error) {
	if !t.valid() {
		return Transaction{}, ErrUninitialized
	}
	if t.status.IsTerminal() {
		return Transaction{}, ErrTerminalStatus
	}
	if t.status != StatusPending && t.status != StatusPendingReference {
		return Transaction{}, ErrInvalidTransition
	}
	if !code.Valid() {
		return Transaction{}, ErrInvalidInput
	}
	next := t
	next.status = target
	next.failureCode = code
	next.updatedAt = now
	return next, nil
}

// ResolveReference sets resolvedReferenceID when status is PENDING or PENDING_REFERENCE.
// Cross-field validation of the referenced transaction belongs to the use case.
func (t Transaction) ResolveReference(internalID string, now time.Time) (Transaction, error) {
	if !t.valid() {
		return Transaction{}, ErrUninitialized
	}
	if internalID == "" {
		return Transaction{}, ErrInvalidInput
	}
	if t.status.IsTerminal() {
		return Transaction{}, ErrTerminalStatus
	}
	if t.status != StatusPending && t.status != StatusPendingReference {
		return Transaction{}, ErrInvalidTransition
	}
	next := t
	next.resolvedReferenceID = internalID
	next.updatedAt = now
	return next, nil
}

func validateAmountForKind(k Kind, amount money.Money) error {
	if !moneyInitialized(amount) {
		return ErrInvalidAmount
	}
	switch k {
	case KindLoss:
		if !amount.IsZero() {
			return ErrInvalidAmount
		}
	case KindBet, KindWin, KindRefund, KindRollback:
		if amount.IsZero() || amount.IsNegative() {
			return ErrInvalidAmount
		}
	case KindOpening:
		if amount.IsNegative() {
			return ErrInvalidAmount
		}
	default:
		return ErrInvalidKind
	}
	return nil
}

func moneyInitialized(m money.Money) bool {
	return m.Currency() != ""
}
