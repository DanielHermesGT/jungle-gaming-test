package wager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	domainwallet "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/clock"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/idgen"
)

// PendingReferenceTTL is how long a PENDING_REFERENCE waits before REJECTED (README §7).
const PendingReferenceTTL = 15 * time.Minute

// UseCase processes external wager transactions (shared by future HTTP/SQS).
type UseCase struct {
	tx      gateway.TxRunner
	read    gateway.ReadQuerier
	wallets gateway.WalletRepository
	ledgers gateway.LedgerRepository
	wagers  gateway.WagerRepository
	ids     idgen.Generator
	clock   clock.Clock
}

func NewUseCase(
	db gateway.DB,
	wallets gateway.WalletRepository,
	ledgers gateway.LedgerRepository,
	wagers gateway.WagerRepository,
	ids idgen.Generator,
	clk clock.Clock,
) *UseCase {
	return &UseCase{
		tx:      db,
		read:    db,
		wallets: wallets,
		ledgers: ledgers,
		wagers:  wagers,
		ids:     ids,
		clock:   clk,
	}
}

// ProcessInput is a provider-facing wager operation.
type ProcessInput struct {
	ProviderID                     string
	ExternalTransactionID          string
	IdempotencyKey                 string
	PlayerID                       string
	WalletID                       string
	RoundID                        string
	GameID                         string
	Kind                           domainwager.Kind
	Amount                         money.Money
	ReferenceExternalTransactionID string
	PayloadHash                    string
}

// ProcessResult is the outcome of Process / ResumePendingReference.
type ProcessResult struct {
	Transaction      domainwager.Transaction
	Balance          *money.Money
	IdempotentReplay bool
}

// Process applies an external wager atomically (wallet + ledger + wager).
func (uc *UseCase) Process(ctx context.Context, in ProcessInput) (ProcessResult, error) {
	if err := validateProcessInput(in); err != nil {
		return ProcessResult{}, err
	}

	var out ProcessResult
	err := uc.tx.WithinTx(ctx, func(q gateway.Querier) error {
		existing, err := uc.wagers.GetByIdempotencyKey(ctx, q, in.IdempotencyKey)
		if err == nil {
			if existing.PayloadHash() != in.PayloadHash {
				return fmt.Errorf("%w: idempotency payload mismatch", usecase.ErrConflict)
			}
			out = ProcessResult{
				Transaction:      existing,
				Balance:          existing.ResultBalance(),
				IdempotentReplay: true,
			}
			return nil
		}
		if !errors.Is(err, gateway.ErrNotFound) {
			return err
		}

		if _, err := uc.wagers.GetByProviderExternal(ctx, q, in.ProviderID, in.ExternalTransactionID); err == nil {
			return fmt.Errorf("%w: provider/external already used", usecase.ErrConflict)
		} else if !errors.Is(err, gateway.ErrNotFound) {
			return err
		}

		w, err := uc.wallets.GetByIDForUpdate(ctx, q, in.WalletID)
		if errors.Is(err, gateway.ErrNotFound) {
			return usecase.ErrNotFound
		}
		if err != nil {
			return err
		}

		now := uc.clock.Now().UTC()
		tx, err := domainwager.NewExternal(domainwager.ExternalParams{
			ID:                           uc.ids.New(),
			Kind:                         in.Kind,
			WalletID:                     in.WalletID,
			PlayerID:                     in.PlayerID,
			ProviderID:                   in.ProviderID,
			ExternalTransactionID:        in.ExternalTransactionID,
			IdempotencyKey:               in.IdempotencyKey,
			PayloadHash:                  in.PayloadHash,
			RoundID:                      in.RoundID,
			GameID:                       in.GameID,
			Amount:                       in.Amount,
			ReferenceExternalTransaction: in.ReferenceExternalTransactionID,
			Now:                          now,
		})
		if err != nil {
			return fmt.Errorf("%w: %v", usecase.ErrInvalidInput, err)
		}

		if w.PlayerID() != in.PlayerID || w.Balance().Currency() != in.Amount.Currency() {
			rejected, err := tx.MarkRejected(domainwager.FailureInvalidAmount, now)
			if err != nil {
				return err
			}
			if err := uc.wagers.Insert(ctx, q, rejected); err != nil {
				return err
			}
			out = ProcessResult{Transaction: rejected}
			return nil
		}

		final, bal, err := uc.applyKind(ctx, q, w, tx, now)
		if err != nil {
			return err
		}
		if err := uc.wagers.Insert(ctx, q, final); err != nil {
			if errors.Is(err, gateway.ErrConflict) {
				return fmt.Errorf("%w: wager unique", usecase.ErrConflict)
			}
			return err
		}
		out = ProcessResult{Transaction: final, Balance: bal}
		return nil
	})
	if err != nil {
		return ProcessResult{}, err
	}
	return out, nil
}

func (uc *UseCase) applyKind(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
) (domainwager.Transaction, *money.Money, error) {
	switch tx.Kind() {
	case domainwager.KindBet:
		return uc.applyDebit(ctx, q, w, tx, now, domainwager.FailureInsufficientFunds)
	case domainwager.KindWin:
		if tx.ReferenceExternalTransaction() != "" {
			ref, err := uc.wagers.GetByProviderExternal(ctx, q, tx.ProviderID(), tx.ReferenceExternalTransaction())
			if errors.Is(err, gateway.ErrNotFound) {
				return uc.awaitReference(tx, now)
			}
			if err != nil {
				return domainwager.Transaction{}, nil, err
			}
			if err := validateReference(tx, ref, false); err != nil {
				rejected, mErr := tx.MarkRejected(errCode(err), now)
				if mErr != nil {
					return domainwager.Transaction{}, nil, mErr
				}
				return rejected, nil, nil
			}
			resolved, err := tx.ResolveReference(ref.ID(), now)
			if err != nil {
				return domainwager.Transaction{}, nil, err
			}
			tx = resolved
		}
		return uc.applyCredit(ctx, q, w, tx, now)
	case domainwager.KindLoss:
		processed, err := tx.MarkProcessed(nil, now)
		if err != nil {
			return domainwager.Transaction{}, nil, err
		}
		bal := w.Balance()
		return processed, &bal, nil
	case domainwager.KindRefund, domainwager.KindRollback:
		return uc.applyReversal(ctx, q, w, tx, now)
	default:
		return domainwager.Transaction{}, nil, fmt.Errorf("%w: kind", usecase.ErrInvalidInput)
	}
}

func (uc *UseCase) applyDebit(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
	insufficientCode domainwager.FailureCode,
) (domainwager.Transaction, *money.Money, error) {
	moved, err := w.Debit(uc.ids.New(), tx.ID(), tx.Amount(), now)
	if errors.Is(err, domainwallet.ErrInsufficientFunds) {
		rejected, mErr := tx.MarkRejected(insufficientCode, now)
		if mErr != nil {
			return domainwager.Transaction{}, nil, mErr
		}
		return rejected, nil, nil
	}
	if err != nil {
		return domainwager.Transaction{}, nil, fmt.Errorf("%w: %v", usecase.ErrInvalidInput, err)
	}
	if err := uc.wallets.Update(ctx, q, moved.Wallet); err != nil {
		return domainwager.Transaction{}, nil, err
	}
	if err := uc.ledgers.Insert(ctx, q, moved.Ledger); err != nil {
		return domainwager.Transaction{}, nil, err
	}
	bal := moved.Wallet.Balance()
	processed, err := tx.MarkProcessed(&bal, now)
	if err != nil {
		return domainwager.Transaction{}, nil, err
	}
	return processed, &bal, nil
}

func (uc *UseCase) applyCredit(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
) (domainwager.Transaction, *money.Money, error) {
	moved, err := w.Credit(uc.ids.New(), tx.ID(), tx.Amount(), now)
	if err != nil {
		return domainwager.Transaction{}, nil, fmt.Errorf("%w: %v", usecase.ErrInvalidInput, err)
	}
	if err := uc.wallets.Update(ctx, q, moved.Wallet); err != nil {
		return domainwager.Transaction{}, nil, err
	}
	if err := uc.ledgers.Insert(ctx, q, moved.Ledger); err != nil {
		return domainwager.Transaction{}, nil, err
	}
	bal := moved.Wallet.Balance()
	processed, err := tx.MarkProcessed(&bal, now)
	if err != nil {
		return domainwager.Transaction{}, nil, err
	}
	return processed, &bal, nil
}

func (uc *UseCase) applyReversal(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
) (domainwager.Transaction, *money.Money, error) {
	ref, err := uc.wagers.GetByProviderExternal(ctx, q, tx.ProviderID(), tx.ReferenceExternalTransaction())
	if errors.Is(err, gateway.ErrNotFound) {
		return uc.awaitReference(tx, now)
	}
	if err != nil {
		return domainwager.Transaction{}, nil, err
	}

	if ref.Status() != domainwager.StatusProcessed {
		if ref.Status().IsTerminal() {
			rejected, mErr := tx.MarkRejected(domainwager.FailureReferenceNotProcessed, now)
			if mErr != nil {
				return domainwager.Transaction{}, nil, mErr
			}
			return rejected, nil, nil
		}
		return uc.awaitReference(tx, now)
	}

	requireBet := tx.Kind() == domainwager.KindRefund
	if err := validateReference(tx, ref, requireBet); err != nil {
		rejected, mErr := tx.MarkRejected(errCode(err), now)
		if mErr != nil {
			return domainwager.Transaction{}, nil, mErr
		}
		return rejected, nil, nil
	}

	dup, err := uc.wagers.GetProcessedReversal(ctx, q, tx.ProviderID(), tx.ReferenceExternalTransaction(), tx.Kind())
	if err == nil && dup.ID() != "" {
		rejected, mErr := tx.MarkRejected(domainwager.FailureDuplicateReversal, now)
		if mErr != nil {
			return domainwager.Transaction{}, nil, mErr
		}
		return rejected, nil, nil
	}
	if err != nil && !errors.Is(err, gateway.ErrNotFound) {
		return domainwager.Transaction{}, nil, err
	}

	resolved, err := tx.ResolveReference(ref.ID(), now)
	if err != nil {
		return domainwager.Transaction{}, nil, err
	}
	tx = resolved

	switch tx.Kind() {
	case domainwager.KindRefund:
		// REFUND credits back a BET debit.
		return uc.applyCredit(ctx, q, w, tx, now)
	case domainwager.KindRollback:
		switch ref.Kind() {
		case domainwager.KindBet:
			return uc.applyCredit(ctx, q, w, tx, now)
		case domainwager.KindWin, domainwager.KindRefund:
			return uc.applyDebit(ctx, q, w, tx, now, domainwager.FailureReversalInsufficientFunds)
		default:
			rejected, mErr := tx.MarkRejected(domainwager.FailureInvalidKind, now)
			if mErr != nil {
				return domainwager.Transaction{}, nil, mErr
			}
			return rejected, nil, nil
		}
	default:
		return domainwager.Transaction{}, nil, fmt.Errorf("%w: kind", usecase.ErrInvalidInput)
	}
}

func (uc *UseCase) awaitReference(tx domainwager.Transaction, now time.Time) (domainwager.Transaction, *money.Money, error) {
	pending, err := tx.AwaitReference(now.Add(PendingReferenceTTL), now)
	if err != nil {
		return domainwager.Transaction{}, nil, err
	}
	return pending, nil, nil
}

// ResumePendingReference continues or expires a PENDING_REFERENCE transaction.
func (uc *UseCase) ResumePendingReference(ctx context.Context, wagerID string) (ProcessResult, error) {
	if wagerID == "" {
		return ProcessResult{}, fmt.Errorf("%w: wagerId", usecase.ErrInvalidInput)
	}

	var out ProcessResult
	err := uc.tx.WithinTx(ctx, func(q gateway.Querier) error {
		head, err := uc.wagers.GetByID(ctx, q, wagerID)
		if errors.Is(err, gateway.ErrNotFound) {
			return usecase.ErrNotFound
		}
		if err != nil {
			return err
		}

		w, err := uc.wallets.GetByIDForUpdate(ctx, q, head.WalletID())
		if errors.Is(err, gateway.ErrNotFound) {
			return usecase.ErrNotFound
		}
		if err != nil {
			return err
		}

		tx, err := uc.wagers.GetByIDForUpdate(ctx, q, wagerID)
		if err != nil {
			return err
		}
		if tx.Status() != domainwager.StatusPendingReference {
			return fmt.Errorf("%w: not pending reference", usecase.ErrInvalidInput)
		}

		now := uc.clock.Now().UTC()
		if !tx.PendingReferenceUntil().IsZero() && !now.Before(tx.PendingReferenceUntil()) {
			rejected, err := tx.MarkRejected(domainwager.FailureReferenceNotFound, now)
			if err != nil {
				return err
			}
			if err := uc.wagers.Update(ctx, q, rejected); err != nil {
				return err
			}
			out = ProcessResult{Transaction: rejected}
			return nil
		}

		// Rebuild a PENDING view for applyReversal/await paths by using domain transitions from PENDING_REFERENCE.
		final, bal, err := uc.resumeApply(ctx, q, w, tx, now)
		if err != nil {
			return err
		}
		if err := uc.wagers.Update(ctx, q, final); err != nil {
			return err
		}
		out = ProcessResult{Transaction: final, Balance: bal}
		return nil
	})
	if err != nil {
		return ProcessResult{}, err
	}
	return out, nil
}

func (uc *UseCase) resumeApply(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
) (domainwager.Transaction, *money.Money, error) {
	ref, err := uc.wagers.GetByProviderExternal(ctx, q, tx.ProviderID(), tx.ReferenceExternalTransaction())
	if errors.Is(err, gateway.ErrNotFound) {
		// Still waiting until TTL.
		return tx, nil, nil
	}
	if err != nil {
		return domainwager.Transaction{}, nil, err
	}

	if ref.Status() != domainwager.StatusProcessed {
		if ref.Status().IsTerminal() {
			rejected, mErr := tx.MarkRejected(domainwager.FailureReferenceNotProcessed, now)
			if mErr != nil {
				return domainwager.Transaction{}, nil, mErr
			}
			return rejected, nil, nil
		}
		return tx, nil, nil
	}

	requireBet := tx.Kind() == domainwager.KindRefund
	if err := validateReference(tx, ref, requireBet); err != nil {
		rejected, mErr := tx.MarkRejected(errCode(err), now)
		if mErr != nil {
			return domainwager.Transaction{}, nil, mErr
		}
		return rejected, nil, nil
	}

	dup, err := uc.wagers.GetProcessedReversal(ctx, q, tx.ProviderID(), tx.ReferenceExternalTransaction(), tx.Kind())
	if err == nil && dup.ID() != "" && dup.ID() != tx.ID() {
		rejected, mErr := tx.MarkRejected(domainwager.FailureDuplicateReversal, now)
		if mErr != nil {
			return domainwager.Transaction{}, nil, mErr
		}
		return rejected, nil, nil
	}
	if err != nil && !errors.Is(err, gateway.ErrNotFound) {
		return domainwager.Transaction{}, nil, err
	}

	resolved, err := tx.ResolveReference(ref.ID(), now)
	if err != nil {
		return domainwager.Transaction{}, nil, err
	}
	tx = resolved

	switch tx.Kind() {
	case domainwager.KindRefund:
		return uc.applyCredit(ctx, q, w, tx, now)
	case domainwager.KindRollback:
		switch ref.Kind() {
		case domainwager.KindBet:
			return uc.applyCredit(ctx, q, w, tx, now)
		case domainwager.KindWin, domainwager.KindRefund:
			return uc.applyDebit(ctx, q, w, tx, now, domainwager.FailureReversalInsufficientFunds)
		default:
			rejected, mErr := tx.MarkRejected(domainwager.FailureInvalidKind, now)
			if mErr != nil {
				return domainwager.Transaction{}, nil, mErr
			}
			return rejected, nil, nil
		}
	case domainwager.KindWin:
		return uc.applyCredit(ctx, q, w, tx, now)
	default:
		return domainwager.Transaction{}, nil, fmt.Errorf("%w: kind", usecase.ErrInvalidInput)
	}
}

// Get returns a wager by internal id.
func (uc *UseCase) Get(ctx context.Context, id string) (domainwager.Transaction, error) {
	if id == "" {
		return domainwager.Transaction{}, fmt.Errorf("%w: id", usecase.ErrInvalidInput)
	}
	tx, err := uc.wagers.GetByID(ctx, uc.read.Querier(), id)
	if errors.Is(err, gateway.ErrNotFound) {
		return domainwager.Transaction{}, usecase.ErrNotFound
	}
	return tx, err
}

// GetByExternal returns a wager by provider + external id.
func (uc *UseCase) GetByExternal(ctx context.Context, providerID, externalID string) (domainwager.Transaction, error) {
	if providerID == "" || externalID == "" {
		return domainwager.Transaction{}, fmt.Errorf("%w: provider/external", usecase.ErrInvalidInput)
	}
	tx, err := uc.wagers.GetByProviderExternal(ctx, uc.read.Querier(), providerID, externalID)
	if errors.Is(err, gateway.ErrNotFound) {
		return domainwager.Transaction{}, usecase.ErrNotFound
	}
	return tx, err
}

func validateProcessInput(in ProcessInput) error {
	if in.ProviderID == "" || in.ExternalTransactionID == "" || in.IdempotencyKey == "" {
		return fmt.Errorf("%w: provider/external/idempotency", usecase.ErrInvalidInput)
	}
	if in.PlayerID == "" || in.WalletID == "" || in.RoundID == "" || in.GameID == "" {
		return fmt.Errorf("%w: player/wallet/round/game", usecase.ErrInvalidInput)
	}
	if in.PayloadHash == "" {
		return fmt.Errorf("%w: payloadHash", usecase.ErrInvalidInput)
	}
	if !in.Kind.IsExternal() {
		return fmt.Errorf("%w: kind", usecase.ErrInvalidInput)
	}
	if in.Amount.Currency() == "" {
		return fmt.Errorf("%w: amount", usecase.ErrInvalidInput)
	}
	return nil
}

type refError struct {
	code domainwager.FailureCode
}

func (e refError) Error() string { return string(e.code) }

func errCode(err error) domainwager.FailureCode {
	var re refError
	if errors.As(err, &re) {
		return re.code
	}
	return domainwager.FailureInvalidAmount
}

func validateReference(tx, ref domainwager.Transaction, requireBet bool) error {
	if requireBet && ref.Kind() != domainwager.KindBet {
		return refError{code: domainwager.FailureInvalidKind}
	}
	if ref.ProviderID() != tx.ProviderID() ||
		ref.PlayerID() != tx.PlayerID() ||
		ref.WalletID() != tx.WalletID() ||
		ref.RoundID() != tx.RoundID() ||
		ref.Amount().Currency() != tx.Amount().Currency() {
		return refError{code: domainwager.FailureInvalidAmount}
	}
	if ref.Amount().Minor() != tx.Amount().Minor() {
		return refError{code: domainwager.FailureInvalidAmount}
	}
	return nil
}
