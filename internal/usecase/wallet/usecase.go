package wallet

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/event"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	domainwallet "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/observability"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/clock"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/idgen"
)

const (
	defaultLedgerLimit = 50
	maxLedgerLimit     = 100
)

// UseCase agrupa os casos de uso do módulo wallet.
type UseCase struct {
	tx      gateway.TxRunner
	read    gateway.ReadQuerier
	wallets gateway.WalletRepository
	ledgers gateway.LedgerRepository
	wagers  gateway.WagerRepository
	outbox  gateway.OutboxRepository
	ids     idgen.Generator
	clock   clock.Clock
}

func NewUseCase(
	db gateway.DB,
	wallets gateway.WalletRepository,
	ledgers gateway.LedgerRepository,
	wagers gateway.WagerRepository,
	outbox gateway.OutboxRepository,
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
		ids:     ids,
		clock:   clk,
	}
}

type OpenInput struct {
	PlayerID       string
	InitialBalance money.Money
}

type WalletView struct {
	ID       string
	PlayerID string
	Balance  money.Money
	Version  int64
}

type ListLedgerInput struct {
	WalletID string
	Cursor   string
	Limit    int
}

type LedgerEntryView struct {
	ID            string
	WalletID      string
	TransactionID string
	Direction     domainwallet.Direction
	Amount        money.Money
	BalanceBefore money.Money
	BalanceAfter  money.Money
	CreatedAt     time.Time
}

type ListLedgerOutput struct {
	Entries    []LedgerEntryView
	NextCursor string
}

type ReconcileOutput struct {
	WalletID          string
	StoredBalance     money.Money
	CalculatedBalance money.Money
	Difference        money.Money
	Consistent        bool
	CheckedEntries    int
}

// Open cria uma carteira. Saldo positivo persiste ledger + OPENING + outbox na mesma TX.
func (uc *UseCase) Open(ctx context.Context, in OpenInput) (WalletView, error) {
	if in.PlayerID == "" {
		return WalletView{}, fmt.Errorf("%w: playerId", usecase.ErrInvalidInput)
	}
	if in.InitialBalance.Currency() == "" || in.InitialBalance.IsNegative() {
		return WalletView{}, fmt.Errorf("%w: initialBalance", usecase.ErrInvalidInput)
	}

	params := domainwallet.OpenParams{
		WalletID:       uc.ids.New(),
		PlayerID:       in.PlayerID,
		InitialBalance: in.InitialBalance,
		Now:            uc.clock.Now().UTC(),
	}
	if !in.InitialBalance.IsZero() {
		params.OpeningTxID = uc.ids.New()
		params.LedgerEntryID = uc.ids.New()
	}

	opened, err := domainwallet.Open(params)
	if err != nil {
		return WalletView{}, fmt.Errorf("%w: %v", usecase.ErrInvalidInput, err)
	}

	var openingTx domainwager.Transaction
	if !in.InitialBalance.IsZero() {
		openingTx, err = domainwager.NewOpening(domainwager.OpeningParams{
			ID:       params.OpeningTxID,
			WalletID: params.WalletID,
			PlayerID: params.PlayerID,
			Amount:   in.InitialBalance,
			Now:      params.Now,
		})
		if err != nil {
			return WalletView{}, fmt.Errorf("%w: opening: %v", usecase.ErrInvalidInput, err)
		}
	}

	err = uc.tx.WithinTx(ctx, func(q gateway.Querier) error {
		if err := uc.wallets.Insert(ctx, q, opened.Wallet); err != nil {
			return err
		}
		if opened.Ledger != nil {
			if err := uc.ledgers.Insert(ctx, q, *opened.Ledger); err != nil {
				return err
			}
		}
		if openingTx.ID() == "" {
			return nil
		}
		if err := uc.wagers.Insert(ctx, q, openingTx); err != nil {
			return err
		}
		now := params.Now
		processed, err := event.NewWagerProcessed(uc.ids.New(), openingTx.ID(), openingTx, now)
		if err != nil {
			return err
		}
		changed, err := event.NewWalletBalanceChanged(
			uc.ids.New(), openingTx.ID(),
			opened.Wallet.ID(), openingTx.ID(),
			opened.Ledger.Direction(),
			opened.Ledger.Amount(),
			opened.Ledger.BalanceBefore(),
			opened.Ledger.BalanceAfter(),
			opened.Wallet.Version(),
			now,
		)
		if err != nil {
			return err
		}
		rec1, err := usecase.OutboxRecordFromEnvelope(processed, now)
		if err != nil {
			return err
		}
		rec2, err := usecase.OutboxRecordFromEnvelope(changed, now)
		if err != nil {
			return err
		}
		return uc.outbox.Insert(ctx, q, rec1, rec2)
	})
	if errors.Is(err, gateway.ErrConflict) {
		return WalletView{}, fmt.Errorf("%w: player/currency", usecase.ErrConflict)
	}
	if err != nil {
		return WalletView{}, err
	}

	return walletView(opened.Wallet), nil
}

// Get retorna a carteira por id.
func (uc *UseCase) Get(ctx context.Context, walletID string) (WalletView, error) {
	if walletID == "" {
		return WalletView{}, fmt.Errorf("%w: walletId", usecase.ErrInvalidInput)
	}
	w, err := uc.wallets.GetByID(ctx, uc.read.Querier(), walletID)
	if errors.Is(err, gateway.ErrNotFound) {
		return WalletView{}, usecase.ErrNotFound
	}
	if err != nil {
		return WalletView{}, err
	}
	return walletView(w), nil
}

// ListLedger lista lançamentos com cursor opaco e ordenação estável.
func (uc *UseCase) ListLedger(ctx context.Context, in ListLedgerInput) (ListLedgerOutput, error) {
	if in.WalletID == "" {
		return ListLedgerOutput{}, fmt.Errorf("%w: walletId", usecase.ErrInvalidInput)
	}
	if _, err := uc.wallets.GetByID(ctx, uc.read.Querier(), in.WalletID); err != nil {
		if errors.Is(err, gateway.ErrNotFound) {
			return ListLedgerOutput{}, usecase.ErrNotFound
		}
		return ListLedgerOutput{}, err
	}

	limit := in.Limit
	if limit == 0 {
		limit = defaultLedgerLimit
	}
	if limit < 0 || limit > maxLedgerLimit {
		return ListLedgerOutput{}, fmt.Errorf("%w: limit", usecase.ErrInvalidInput)
	}

	var cursorAt time.Time
	var cursorID string
	if in.Cursor != "" {
		var err error
		cursorAt, cursorID, err = decodeLedgerCursor(in.Cursor)
		if err != nil {
			return ListLedgerOutput{}, err
		}
	}

	rows, err := uc.ledgers.ListByWalletIDPage(ctx, uc.read.Querier(), in.WalletID, cursorAt, cursorID, limit+1)
	if err != nil {
		return ListLedgerOutput{}, err
	}

	out := ListLedgerOutput{Entries: make([]LedgerEntryView, 0, min(len(rows), limit))}
	for i, e := range rows {
		if i == limit {
			last := out.Entries[len(out.Entries)-1]
			out.NextCursor = encodeLedgerCursor(last.CreatedAt, last.ID)
			break
		}
		out.Entries = append(out.Entries, LedgerEntryView{
			ID:            e.ID(),
			WalletID:      e.WalletID(),
			TransactionID: e.TransactionID(),
			Direction:     e.Direction(),
			Amount:        e.Amount(),
			BalanceBefore: e.BalanceBefore(),
			BalanceAfter:  e.BalanceAfter(),
			CreatedAt:     e.CreatedAt(),
		})
	}
	return out, nil
}

// Reconcile compara saldo armazenado com a reconstrução do ledger. Não altera saldo.
// TODO(futuro): métrica de divergência (README §12).
func (uc *UseCase) Reconcile(ctx context.Context, walletID string) (ReconcileOutput, error) {
	if walletID == "" {
		return ReconcileOutput{}, fmt.Errorf("%w: walletId", usecase.ErrInvalidInput)
	}

	var out ReconcileOutput
	err := uc.tx.WithinTx(ctx, func(q gateway.Querier) error {
		w, err := uc.wallets.GetByIDForUpdate(ctx, q, walletID)
		if errors.Is(err, gateway.ErrNotFound) {
			return usecase.ErrNotFound
		}
		if err != nil {
			return err
		}

		entries, err := uc.ledgers.ListByWalletID(ctx, q, walletID)
		if err != nil {
			return err
		}

		calculated, err := money.Zero(w.Balance().Currency())
		if err != nil {
			return err
		}
		for _, e := range entries {
			switch e.Direction() {
			case domainwallet.DirectionCredit:
				calculated, err = calculated.Add(e.Amount())
			case domainwallet.DirectionDebit:
				calculated, err = calculated.Sub(e.Amount())
			default:
				return fmt.Errorf("%w: ledger direction", usecase.ErrInvalidInput)
			}
			if err != nil {
				return err
			}
		}

		diff, err := w.Balance().Sub(calculated)
		if err != nil {
			return err
		}

		out = ReconcileOutput{
			WalletID:          walletID,
			StoredBalance:     w.Balance(),
			CalculatedBalance: calculated,
			Difference:        diff,
			Consistent:        diff.IsZero(),
			CheckedEntries:    len(entries),
		}
		return nil
	})
	if err != nil {
		return ReconcileOutput{}, err
	}

	if !out.Consistent {
		observability.ReconcileDivergences.Add(1)
		slog.Warn("wallet reconciliation inconsistent",
			"walletId", out.WalletID,
			"checkedEntries", out.CheckedEntries,
		)
	}
	return out, nil
}

func walletView(w domainwallet.Wallet) WalletView {
	return WalletView{
		ID:       w.ID(),
		PlayerID: w.PlayerID(),
		Balance:  w.Balance(),
		Version:  w.Version(),
	}
}

type ledgerCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"i"`
}

func encodeLedgerCursor(createdAt time.Time, id string) string {
	raw, err := json.Marshal(ledgerCursor{CreatedAt: createdAt.UTC(), ID: id})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeLedgerCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("%w: cursor", usecase.ErrInvalidInput)
	}
	var c ledgerCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return time.Time{}, "", fmt.Errorf("%w: cursor", usecase.ErrInvalidInput)
	}
	if c.ID == "" || c.CreatedAt.IsZero() {
		return time.Time{}, "", fmt.Errorf("%w: cursor", usecase.ErrInvalidInput)
	}
	return c.CreatedAt.UTC(), c.ID, nil
}
