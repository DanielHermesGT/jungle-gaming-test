package wager

import (
	"context"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/event"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	domainwallet "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
)

func (uc *UseCase) emitForResult(
	ctx context.Context,
	q gateway.Querier,
	out ProcessResult,
	ledger *domainwallet.LedgerEntry,
	now time.Time,
) error {
	if out.IdempotentReplay {
		return nil
	}
	tx := out.Transaction
	corr := tx.ID()
	var records []gateway.OutboxRecord

	switch tx.Status() {
	case domainwager.StatusProcessed:
		env, err := event.NewWagerProcessed(uc.ids.New(), corr, tx, now)
		if err != nil {
			return err
		}
		rec, err := usecase.OutboxRecordFromEnvelope(env, now)
		if err != nil {
			return err
		}
		records = append(records, rec)
		if ledger != nil {
			w, err := uc.wallets.GetByID(ctx, q, ledger.WalletID())
			if err != nil {
				return err
			}
			env2, err := event.NewWalletBalanceChanged(
				uc.ids.New(), corr,
				ledger.WalletID(), ledger.TransactionID(),
				ledger.Direction(), ledger.Amount(),
				ledger.BalanceBefore(), ledger.BalanceAfter(),
				w.Version(), now,
			)
			if err != nil {
				return err
			}
			rec2, err := usecase.OutboxRecordFromEnvelope(env2, now)
			if err != nil {
				return err
			}
			records = append(records, rec2)
		}
	case domainwager.StatusRejected:
		env, err := event.NewWagerRejected(uc.ids.New(), corr, tx, now)
		if err != nil {
			return err
		}
		rec, err := usecase.OutboxRecordFromEnvelope(env, now)
		if err != nil {
			return err
		}
		records = append(records, rec)
	case domainwager.StatusPendingReference:
		env, err := event.NewWagerPendingReference(uc.ids.New(), corr, tx, now)
		if err != nil {
			return err
		}
		rec, err := usecase.OutboxRecordFromEnvelope(env, now)
		if err != nil {
			return err
		}
		records = append(records, rec)
	default:
		return nil
	}
	if len(records) == 0 {
		return nil
	}
	return uc.outbox.Insert(ctx, q, records...)
}
