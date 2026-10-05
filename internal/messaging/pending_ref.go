package messaging

import (
	"context"
	"log/slog"
	"sync"
	"time"

	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
)

const pendingRefPollInterval = 2 * time.Second
const pendingRefBatch = 50

type pendingResumer interface {
	ResumePendingReference(ctx context.Context, wagerID string) (usecasewager.ProcessResult, error)
}

// PendingRefWorker polls PENDING_REFERENCE rows and resumes them with per-id exponential backoff.
type PendingRefWorker struct {
	db     gateway.DB
	wagers gateway.WagerRepository
	uc     pendingResumer

	mu       sync.Mutex
	nextTry  map[string]time.Time
	attempts map[string]int
	wg       sync.WaitGroup
}

func NewPendingRefWorker(
	db gateway.DB,
	wagers gateway.WagerRepository,
	uc *usecasewager.UseCase,
) *PendingRefWorker {
	return &PendingRefWorker{
		db:       db,
		wagers:   wagers,
		uc:       uc,
		nextTry:  make(map[string]time.Time),
		attempts: make(map[string]int),
	}
}

func (w *PendingRefWorker) Run(ctx context.Context) {
	slog.Info("messaging: pending-reference worker started")
	w.wg.Add(1)
	defer w.wg.Done()
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

// Wait blocks until Run exits (after ctx cancel).
func (w *PendingRefWorker) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *PendingRefWorker) tick(ctx context.Context) error {
	q := w.db.Querier()
	list, err := w.wagers.ListPendingReference(ctx, q, pendingRefBatch)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, tx := range list {
		if tx.Status() != domainwager.StatusPendingReference {
			continue
		}
		if !w.due(tx.ID(), now) {
			continue
		}
		out, err := w.uc.ResumePendingReference(ctx, tx.ID())
		if err != nil {
			slog.Warn("messaging: resume pending ref failed", "wagerId", tx.ID(), "err", err)
			w.schedule(tx.ID(), now)
			continue
		}
		if out.Transaction.Status() == domainwager.StatusPendingReference {
			w.schedule(tx.ID(), now)
			continue
		}
		w.clear(tx.ID())
	}
	return nil
}

func (w *PendingRefWorker) due(id string, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	next, ok := w.nextTry[id]
	return !ok || !now.Before(next)
}

func (w *PendingRefWorker) schedule(id string, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := w.attempts[id] + 1
	w.attempts[id] = n
	// 1s, 2s, 4s, … capped at 5m (README §7 exponential backoff)
	sec := 1 << min(n, 8)
	if sec > 300 {
		sec = 300
	}
	w.nextTry[id] = now.Add(time.Duration(sec) * time.Second)
}

func (w *PendingRefWorker) clear(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.nextTry, id)
	delete(w.attempts, id)
}
