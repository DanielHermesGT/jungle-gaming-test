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
	"github.com/DanielHermesGT/jungle-gaming-test/internal/observability"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/clock"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/idgen"
)

// PendingReferenceTTL is how long a PENDING_REFERENCE waits before REJECTED (README §7).
const PendingReferenceTTL = 15 * time.Minute

// UseCase processes external wager transactions (HTTP and SQS share Process).
type UseCase struct {
	tx      gateway.TxRunner
	read    gateway.ReadQuerier
	wallets gateway.WalletRepository
	ledgers gateway.LedgerRepository
	wagers  gateway.WagerRepository
	outbox  gateway.OutboxRepository
	inbox   gateway.InboxRepository
	ids     idgen.Generator
	clock   clock.Clock
}

func NewUseCase(
	db gateway.DB,
	wallets gateway.WalletRepository,
	ledgers gateway.LedgerRepository,
	wagers gateway.WagerRepository,
	outbox gateway.OutboxRepository,
	inbox gateway.InboxRepository,
	ids idgen.Generator,
	clk clock.Clock,
) *UseCase {
	return &UseCase{
		tx:      db,
		read:    db,
		wallets: wallets,
		ledgers: ledgers,
		wagers:  wagers,
		outbox:  outbox,
		inbox:   inbox,
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

// ProcessResult is the outcome of Process / ResumePendingReference / ProcessFromQueue.
type ProcessResult struct {
	Transaction      domainwager.Transaction
	Balance          *money.Money
	IdempotentReplay bool
}

type applyOutcome struct {
	tx     domainwager.Transaction
	bal    *money.Money
	ledger *domainwallet.LedgerEntry
}

// Process applies an external wager atomically (wallet + ledger + wager + outbox).
func (uc *UseCase) Process(ctx context.Context, in ProcessInput) (ProcessResult, error) {
	if err := validateProcessInput(in); err != nil {
		return ProcessResult{}, err
	}

	var out ProcessResult
	err := uc.tx.WithinTx(ctx, func(q gateway.Querier) error {
		var ledger *domainwallet.LedgerEntry
		var err error
		out, ledger, err = uc.processInTx(ctx, q, in)
		if err != nil {
			return err
		}
		return uc.emitForResult(ctx, q, out, ledger, uc.clock.Now().UTC())
	})
	if err != nil {
		if errors.Is(err, usecase.ErrConflict) {
			observability.Conflicts.Add(1)
		}
		return ProcessResult{}, err
	}
	recordProcessMetrics(out)
	return out, nil
}

// ProcessFromQueue processes a wager from SQS with durable inbox dedup in the same TX.
func (uc *UseCase) ProcessFromQueue(
	ctx context.Context,
	consumerName, messageID, payloadHash string,
	in ProcessInput,
) (ProcessResult, error) {
	if consumerName == "" || messageID == "" || payloadHash == "" {
		return ProcessResult{}, fmt.Errorf("%w: inbox keys", usecase.ErrInvalidInput)
	}
	if err := validateProcessInput(in); err != nil {
		return ProcessResult{}, err
	}

	var out ProcessResult
	err := uc.tx.WithinTx(ctx, func(q gateway.Querier) error {
		now := uc.clock.Now().UTC()
		existing, err := uc.inbox.Get(ctx, q, consumerName, messageID)
		if err == nil {
			if existing.PayloadHash != payloadHash {
				return fmt.Errorf("%w: inbox payload hash mismatch", usecase.ErrPermanent)
			}
			if existing.CompletedAt != nil {
				// Replay: load financial idempotent result if present.
				tx, gErr := uc.wagers.GetByIdempotencyKey(ctx, q, in.IdempotencyKey)
				if gErr != nil {
					return gErr
				}
				out = ProcessResult{
					Transaction:      tx,
					Balance:          tx.ResultBalance(),
					IdempotentReplay: true,
				}
				return nil
			}
		} else if !errors.Is(err, gateway.ErrNotFound) {
			return err
		} else if err := uc.inbox.InsertReceived(ctx, q, gateway.InboxMessage{
			ConsumerName: consumerName,
			MessageID:    messageID,
			PayloadHash:  payloadHash,
			ReceivedAt:   now,
		}); err != nil {
			// Concurrent claim of the same SQS message — retry on next delivery.
			if errors.Is(err, gateway.ErrConflict) {
				return fmt.Errorf("%w: inbox race", err)
			}
			return err
		}

		var ledger *domainwallet.LedgerEntry
		out, ledger, err = uc.processInTx(ctx, q, in)
		if err != nil {
			return err
		}
		if err := uc.emitForResult(ctx, q, out, ledger, now); err != nil {
			return err
		}
		return uc.inbox.MarkCompleted(ctx, q, consumerName, messageID, now)
	})
	if err != nil {
		if errors.Is(err, usecase.ErrConflict) {
			observability.Conflicts.Add(1)
		}
		return ProcessResult{}, err
	}
	recordProcessMetrics(out)
	return out, nil
}

func (uc *UseCase) processInTx(
	ctx context.Context,
	q gateway.Querier,
	in ProcessInput,
) (ProcessResult, *domainwallet.LedgerEntry, error) {
	existing, err := uc.wagers.GetByIdempotencyKey(ctx, q, in.IdempotencyKey)
	if err == nil {
		if existing.PayloadHash() != in.PayloadHash {
			return ProcessResult{}, nil, fmt.Errorf("%w: idempotency payload mismatch", usecase.ErrConflict)
		}
		return ProcessResult{
			Transaction:      existing,
			Balance:          existing.ResultBalance(),
			IdempotentReplay: true,
		}, nil, nil
	}
	if !errors.Is(err, gateway.ErrNotFound) {
		return ProcessResult{}, nil, err
	}

	if _, err := uc.wagers.GetByProviderExternal(ctx, q, in.ProviderID, in.ExternalTransactionID); err == nil {
		return ProcessResult{}, nil, fmt.Errorf("%w: provider/external already used", usecase.ErrConflict)
	} else if !errors.Is(err, gateway.ErrNotFound) {
		return ProcessResult{}, nil, err
	}

	w, err := uc.wallets.GetByIDForUpdate(ctx, q, in.WalletID)
	if errors.Is(err, gateway.ErrNotFound) {
		return ProcessResult{}, nil, usecase.ErrNotFound
	}
	if err != nil {
		return ProcessResult{}, nil, err
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
		return ProcessResult{}, nil, fmt.Errorf("%w: %v", usecase.ErrInvalidInput, err)
	}

	if w.PlayerID() != in.PlayerID || w.Balance().Currency() != in.Amount.Currency() {
		rejected, err := tx.MarkRejected(domainwager.FailureInvalidAmount, now)
		if err != nil {
			return ProcessResult{}, nil, err
		}
		if err := uc.wagers.Insert(ctx, q, rejected); err != nil {
			return ProcessResult{}, nil, err
		}
		return ProcessResult{Transaction: rejected}, nil, nil
	}

	outcome, err := uc.applyKind(ctx, q, w, tx, now)
	if err != nil {
		return ProcessResult{}, nil, err
	}
	if err := uc.wagers.Insert(ctx, q, outcome.tx); err != nil {
		if errors.Is(err, gateway.ErrConflict) {
			return ProcessResult{}, nil, fmt.Errorf("%w: wager unique", usecase.ErrConflict)
		}
		return ProcessResult{}, nil, err
	}
	return ProcessResult{Transaction: outcome.tx, Balance: outcome.bal}, outcome.ledger, nil
}

func (uc *UseCase) applyKind(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
) (applyOutcome, error) {
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
				return applyOutcome{}, err
			}
			if err := validateReference(tx, ref, false); err != nil {
				rejected, mErr := tx.MarkRejected(errCode(err), now)
				if mErr != nil {
					return applyOutcome{}, mErr
				}
				return applyOutcome{tx: rejected}, nil
			}
			resolved, err := tx.ResolveReference(ref.ID(), now)
			if err != nil {
				return applyOutcome{}, err
			}
			tx = resolved
		}
		return uc.applyCredit(ctx, q, w, tx, now)
	case domainwager.KindLoss:
		processed, err := tx.MarkProcessed(nil, now)
		if err != nil {
			return applyOutcome{}, err
		}
		bal := w.Balance()
		return applyOutcome{tx: processed, bal: &bal}, nil
	case domainwager.KindRefund, domainwager.KindRollback:
		return uc.applyReversal(ctx, q, w, tx, now)
	default:
		return applyOutcome{}, fmt.Errorf("%w: kind", usecase.ErrInvalidInput)
	}
}

func (uc *UseCase) applyDebit(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
	insufficientCode domainwager.FailureCode,
) (applyOutcome, error) {
	moved, err := w.Debit(uc.ids.New(), tx.ID(), tx.Amount(), now)
	if errors.Is(err, domainwallet.ErrInsufficientFunds) {
		rejected, mErr := tx.MarkRejected(insufficientCode, now)
		if mErr != nil {
			return applyOutcome{}, mErr
		}
		return applyOutcome{tx: rejected}, nil
	}
	if err != nil {
		return applyOutcome{}, fmt.Errorf("%w: %v", usecase.ErrInvalidInput, err)
	}
	if err := uc.wallets.Update(ctx, q, moved.Wallet); err != nil {
		return applyOutcome{}, err
	}
	if err := uc.ledgers.Insert(ctx, q, moved.Ledger); err != nil {
		return applyOutcome{}, err
	}
	bal := moved.Wallet.Balance()
	ledger := moved.Ledger
	processed, err := tx.MarkProcessed(&bal, now)
	if err != nil {
		return applyOutcome{}, err
	}
	return applyOutcome{tx: processed, bal: &bal, ledger: &ledger}, nil
}

func (uc *UseCase) applyCredit(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
) (applyOutcome, error) {
	moved, err := w.Credit(uc.ids.New(), tx.ID(), tx.Amount(), now)
	if err != nil {
		return applyOutcome{}, fmt.Errorf("%w: %v", usecase.ErrInvalidInput, err)
	}
	if err := uc.wallets.Update(ctx, q, moved.Wallet); err != nil {
		return applyOutcome{}, err
	}
	if err := uc.ledgers.Insert(ctx, q, moved.Ledger); err != nil {
		return applyOutcome{}, err
	}
	bal := moved.Wallet.Balance()
	ledger := moved.Ledger
	processed, err := tx.MarkProcessed(&bal, now)
	if err != nil {
		return applyOutcome{}, err
	}
	return applyOutcome{tx: processed, bal: &bal, ledger: &ledger}, nil
}

func (uc *UseCase) applyReversal(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
) (applyOutcome, error) {
	ref, err := uc.wagers.GetByProviderExternal(ctx, q, tx.ProviderID(), tx.ReferenceExternalTransaction())
	if errors.Is(err, gateway.ErrNotFound) {
		return uc.awaitReference(tx, now)
	}
	if err != nil {
		return applyOutcome{}, err
	}

	if ref.Status() != domainwager.StatusProcessed {
		if ref.Status().IsTerminal() {
			rejected, mErr := tx.MarkRejected(domainwager.FailureReferenceNotProcessed, now)
			if mErr != nil {
				return applyOutcome{}, mErr
			}
			return applyOutcome{tx: rejected}, nil
		}
		return uc.awaitReference(tx, now)
	}

	requireBet := tx.Kind() == domainwager.KindRefund
	if err := validateReference(tx, ref, requireBet); err != nil {
		rejected, mErr := tx.MarkRejected(errCode(err), now)
		if mErr != nil {
			return applyOutcome{}, mErr
		}
		return applyOutcome{tx: rejected}, nil
	}

	if dup, err := uc.findDuplicateReversal(ctx, q, tx, ref); err != nil {
		return applyOutcome{}, err
	} else if dup != nil {
		rejected, mErr := tx.MarkRejected(domainwager.FailureDuplicateReversal, now)
		if mErr != nil {
			return applyOutcome{}, mErr
		}
		return applyOutcome{tx: rejected}, nil
	}

	resolved, err := tx.ResolveReference(ref.ID(), now)
	if err != nil {
		return applyOutcome{}, err
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
				return applyOutcome{}, mErr
			}
			return applyOutcome{tx: rejected}, nil
		}
	default:
		return applyOutcome{}, fmt.Errorf("%w: kind", usecase.ErrInvalidInput)
	}
}

func (uc *UseCase) awaitReference(tx domainwager.Transaction, now time.Time) (applyOutcome, error) {
	pending, err := tx.AwaitReference(now.Add(PendingReferenceTTL), now)
	if err != nil {
		return applyOutcome{}, err
	}
	return applyOutcome{tx: pending}, nil
}

// findDuplicateReversal returns an existing PROCESSED reversal that would double-credit
// or repeat the same kind. For BET references, REFUND and ROLLBACK are mutually exclusive
// (both credit the original debit). For WIN/REFUND references, only same-kind ROLLBACK counts.
func (uc *UseCase) findDuplicateReversal(
	ctx context.Context,
	q gateway.Querier,
	tx, ref domainwager.Transaction,
) (*domainwager.Transaction, error) {
	kinds := []domainwager.Kind{tx.Kind()}
	if ref.Kind() == domainwager.KindBet {
		kinds = []domainwager.Kind{domainwager.KindRefund, domainwager.KindRollback}
	}
	dup, err := uc.wagers.GetProcessedReversal(ctx, q, tx.ProviderID(), tx.ReferenceExternalTransaction(), kinds...)
	if errors.Is(err, gateway.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if dup.ID() == "" || dup.ID() == tx.ID() {
		return nil, nil
	}
	return &dup, nil
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
			return uc.emitForResult(ctx, q, out, nil, now)
		}

		outcome, err := uc.resumeApply(ctx, q, w, tx, now)
		if err != nil {
			return err
		}
		if outcome.tx.ID() == tx.ID() && outcome.tx.Status() == domainwager.StatusPendingReference {
			// Still waiting; no status change → no outbox.
			out = ProcessResult{Transaction: outcome.tx, Balance: outcome.bal}
			return nil
		}
		if err := uc.wagers.Update(ctx, q, outcome.tx); err != nil {
			return err
		}
		out = ProcessResult{Transaction: outcome.tx, Balance: outcome.bal}
		return uc.emitForResult(ctx, q, out, outcome.ledger, now)
	})
	if err != nil {
		return ProcessResult{}, err
	}
	recordProcessMetrics(out)
	return out, nil
}

func (uc *UseCase) resumeApply(
	ctx context.Context,
	q gateway.Querier,
	w domainwallet.Wallet,
	tx domainwager.Transaction,
	now time.Time,
) (applyOutcome, error) {
	ref, err := uc.wagers.GetByProviderExternal(ctx, q, tx.ProviderID(), tx.ReferenceExternalTransaction())
	if errors.Is(err, gateway.ErrNotFound) {
		return applyOutcome{tx: tx}, nil
	}
	if err != nil {
		return applyOutcome{}, err
	}

	if ref.Status() != domainwager.StatusProcessed {
		if ref.Status().IsTerminal() {
			rejected, mErr := tx.MarkRejected(domainwager.FailureReferenceNotProcessed, now)
			if mErr != nil {
				return applyOutcome{}, mErr
			}
			return applyOutcome{tx: rejected}, nil
		}
		return applyOutcome{tx: tx}, nil
	}

	requireBet := tx.Kind() == domainwager.KindRefund
	if err := validateReference(tx, ref, requireBet); err != nil {
		rejected, mErr := tx.MarkRejected(errCode(err), now)
		if mErr != nil {
			return applyOutcome{}, mErr
		}
		return applyOutcome{tx: rejected}, nil
	}

	if dup, err := uc.findDuplicateReversal(ctx, q, tx, ref); err != nil {
		return applyOutcome{}, err
	} else if dup != nil {
		rejected, mErr := tx.MarkRejected(domainwager.FailureDuplicateReversal, now)
		if mErr != nil {
			return applyOutcome{}, mErr
		}
		return applyOutcome{tx: rejected}, nil
	}

	resolved, err := tx.ResolveReference(ref.ID(), now)
	if err != nil {
		return applyOutcome{}, err
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
				return applyOutcome{}, mErr
			}
			return applyOutcome{tx: rejected}, nil
		}
	case domainwager.KindWin:
		return uc.applyCredit(ctx, q, w, tx, now)
	default:
		return applyOutcome{}, fmt.Errorf("%w: kind", usecase.ErrInvalidInput)
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

func recordProcessMetrics(out ProcessResult) {
	if out.IdempotentReplay {
		observability.IdempotentReplays.Add(1)
		return
	}
	switch out.Transaction.Status() {
	case domainwager.StatusProcessed:
		observability.WagerProcessed.Add(1)
	case domainwager.StatusRejected:
		observability.WagerRejected.Add(1)
	case domainwager.StatusPendingReference:
		observability.WagerPendingRef.Add(1)
	}
}
