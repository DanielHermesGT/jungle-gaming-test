package messaging

import (
	"context"
	"log/slog"
	"time"

	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
)

const pendingRefPollInterval = 5 * time.Second
const pendingRefBatch = 50

type pendingResumer interface {
	ResumePendingReference(ctx context.Context, wagerID string) (usecasewager.ProcessResult, error)
}

// PendingRefWorker polls PENDING_REFERENCE rows and resumes them.
type PendingRefWorker struct {
	db     gateway.DB
	wagers gateway.WagerRepository
	uc     pendingResumer
}

func NewPendingRefWorker(
	db gateway.DB,
	wagers gateway.WagerRepository,
	uc *usecasewager.UseCase,
) *PendingRefWorker {
	return &PendingRefWorker{db: db, wagers: wagers, uc: uc}
}

func (w *PendingRefWorker) Run(ctx context.Context) {
	slog.Info("messaging: pending-reference worker started")
	for {
		if ctx.Err() != nil {
			return
		}
		if err := w.tick(ctx); err != nil {
			slog.Error("messaging: pending-ref tick failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pendingRefPollInterval):
		}
	}
}

func (w *PendingRefWorker) tick(ctx context.Context) error {
	q := w.db.Querier()
	list, err := w.wagers.ListPendingReference(ctx, q, pendingRefBatch)
	if err != nil {
		return err
	}
	for _, tx := range list {
		if tx.Status() != domainwager.StatusPendingReference {
			continue
		}
		if _, err := w.uc.ResumePendingReference(ctx, tx.ID()); err != nil {
			slog.Warn("messaging: resume pending ref failed", "wagerId", tx.ID(), "err", err)
		}
	}
	return nil
}
