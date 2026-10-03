package gateway

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier é "por onde o SQL roda": Exec/Query/QueryRow.
//
// Na prática pode ser uma destas duas coisas (ambas implementam esta interface):
//   - *pgxpool.Pool — conexão do pool, sem transação explícita
//   - pgx.Tx        — transação aberta (BEGIN … COMMIT/ROLLBACK)
//
// Quando usar cada uma:
//   - Pool: leituras (Get, listar ledger) em que não precisamos de TX
//   - Tx:   escritas que precisam ser atômicas (ex.: wallet + ledger no mesmo commit)
//
// Os repositórios recebem q por parâmetro; quem chama (use case) decide se passa
// o pool ou a Tx. Assim vários Insert/Update compartilham a mesma sessão SQL.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxRunner abre uma transação e entrega a Tx como Querier para o callback.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(q Querier) error) error
}

// ReadQuerier entrega o pool como Querier para leituras fora de TX.
type ReadQuerier interface {
	Querier() Querier
}

// DB junta TxRunner + ReadQuerier para o use case (escrever em TX e ler no pool).
type DB interface {
	TxRunner
	ReadQuerier
}
