# Architecture

Decisões técnicas da solução. Esta seção começa pelo domínio monetário; demais tópicos serão registrados conforme forem implementados.

## Money

### Representação

`Money` é um value object imutável em `internal/domain/money`.

- Valor interno: `int64` em **unidades mínimas** (centavos), escala fixa de **2**.
- Moeda: código ISO 4217 no formato `^[A-Z]{3}$` (sem catálogo completo de códigos).
- Não teve uso de `float32`/`float64` nem biblioteca decimal.

Motivo: escala 2 cobre o contrato do desafio (`"25.00"`), a aritmética fica explícita, e evita dependência extra.

### Limites

Com `int64` e escala 2, o intervalo representável é aproximadamente:

- mínimo: `-92233720368547758.08`
- máximo: `92233720368547758.07`

Overflow em parsing, soma, subtração e negação retorna `money.ErrOverflow`.

### Mapeamento

| Camada | Forma |
| --- | --- |
| API / JSON | `{"amount":"25.00","currency":"BRL"}` (strings) |
| Domínio | `minor int64` + `currency string` |
| Persistência | `BIGINT` (minor) + `CHAR(3)` (currency) |

### Entrada externa (`Parse`)

Aceita somente amount no formato rígido: dígitos + `.` + exatamente duas casas (`"25.00"`, `"0.00"`).

Rejeita, sem arredondar:

- vazio, sinais (`+`/`-`), `NaN`, `Infinity`
- notação científica
- `"25"`, `"25.0"`, `"25.001"`, `"25."` e similares
- moeda inválida (tamanho ≠ 3 ou não `A-Z`)

Não há normalização de formas equivalentes. O amount que entra no hash de idempotência futuro já precisa chegar canônico como `"XX.YY"`; HTTP e SQS compartilham a mesma regra via `Parse`.

### Valores negativos

- `Parse` / `UnmarshalJSON` (entrada externa): negativos são inválidos.
- Uso interno (`FromMinor`, `Sub`, `Neg`): negativos são permitidos para diferenças e cálculos.
- Saldo de carteira não-negativo é responsabilidade do agregado `Wallet` e das constraints do banco.

### Erros

Erros sentinela classificáveis com `errors.Is`:

- `ErrInvalidAmount`
- `ErrInvalidCurrency`
- `ErrCurrencyMismatch`
- `ErrOverflow`
- `ErrUninitialized` (zero-value `Money{}`)

### Serialização

`AmountString` e `MarshalJSON` sempre emitem duas casas decimais. `UnmarshalJSON` reutiliza `Parse`.

## Wallet

### Agregado

`Wallet` em `internal/domain/wallet` é a raiz do agregado financeiro.

- Campos: `id`, `playerId`, saldo (`money.Money`), `version`, `createdAt`, `updatedAt`.
- O saldo só muda por `Open` (saldo inicial), `Credit` e `Debit`.
- `WalletFromPersisted` / `LedgerEntryFromPersisted` reconstroem estado persistido sem reaplicar movimentos nem gerar ledger.
- IDs e timestamps são fornecidos pela camada de aplicação (o domínio não gera UUID).

### Abertura

| Saldo inicial | Efeito |
| --- | --- |
| `0.00` | Wallet com `version = 1`, sem lançamento de ledger |
| `> 0` | Wallet com `version = 1` + um `CREDIT` de abertura no ledger (`balanceBefore = 0`) |

A transação de negócio `OPENING` / eventos de outbox ficam na application layer; o domínio wallet apenas materializa saldo + ledger de abertura.

### Ledger

`LedgerEntry` é imutável. Cada `Credit`/`Debit` bem-sucedido produz exatamente um lançamento com:

- direção `CREDIT` ou `DEBIT`
- `balanceAfter = balanceBefore ± amount` (validado na construção)
- vínculo a `walletId` e `transactionId`

`LOSS` e rejeições (ainda não modelados aqui) não devem chamar `Credit`/`Debit`.

Movimentos exigem amount **> 0** e a mesma moeda da carteira. Débito com saldo insuficiente retorna `ErrInsufficientFunds` sem alterar o agregado.

`version` inicia em `1` e incrementa **somente** quando o saldo muda (`Credit`/`Debit`).

### Organização de pacotes

Inspirada em clean architecture (referência interna `pratico-ms-vEDA`), adaptada ao README:

```text
internal/
  domain/       # Money, Wallet, LedgerEntry (mantido; não "entity" — Money é VO)
  gateway/      # ports (interfaces de repositório + Querier/TxRunner)
  database/     # adapters Postgres (pgx, SQL explícito)
  usecase/      # um pacote por módulo (ex.: wallet/)
pkg/            # idgen, clock
cmd/            # entrypoints (HTTP futuro)
```

Wallet e `LedgerEntry` ficam no **mesmo** pacote `internal/domain/wallet`. O ledger não é agregado independente; nasce só com mudança de saldo. Persistência: duas tabelas e dois repos em `internal/database`.

### Concorrência

O README (§8) permite pessimista, otimista com retry, update atômico condicionado ou combinação. Coordenação **por carteira**; lock global é proibido.

#### Pessimista vs otimista (resumo)

- **Pessimista:** `SELECT … FOR UPDATE` bloqueia a linha da carteira antes de alterar; outros writers na mesma wallet esperam. Simples e adequado à disputa 100 vs 2×80.
- **Otimista:** lê sem lock e grava com `WHERE version = $antiga`; se 0 rows, houve conflito → retry. Melhor quando conflito é raro; sob contenção na mesma wallet gera muitos retries.
- **Update atômico:** `UPDATE … WHERE balance_minor >= $amount` como rede de segurança SQL — não substitui o agregado de domínio.

#### Escolha adotada

**Locking pessimista por carteira** (`SELECT … FROM wallets WHERE id = $1 FOR UPDATE`):

1. Lock apenas na linha da wallet alvo (carteiras distintas seguem em paralelo)
2. TX curta: lock → domínio (`Credit`/`Debit`) → `UPDATE` wallet + `INSERT` ledger → `COMMIT`
3. Campo `version`: incrementado no domínio a cada mudança de saldo; persistido para auditoria. O mecanismo primário anti lost-update entre writers é o `FOR UPDATE`, não retry otimista
4. Constraints no DB como fonte da verdade adicional (abaixo)

Não usamos motor de retry otimista nem fila/lock global por processo.

#### Atomicidade (obrigatório — não esquecer)

```text
BEGIN
  -- FOR UPDATE na wallet quando for alterar
  -- INSERT/UPDATE wallet
  -- INSERT ledger (sempre junto com mudança de saldo)
  -- TODO(futuro): INSERT wager_transaction / inbox / outbox na MESMA TX
COMMIT
```

Wallet + ledger da mesma operação financeira **nunca** em commits separados. Quando existirem inbox/outbox/wager, entram neste mesmo `BEGIN…COMMIT`.

#### Constraints (fonte da verdade no DB)

| Constraint | Onde | Por quê |
| --- | --- | --- |
| `UNIQUE (player_id, currency)` | `wallets` | uma carteira por jogador+moeda |
| `UNIQUE (wallet_id, transaction_id)` | `wallet_ledger_entries` | um lançamento por transação na carteira |
| `CHECK (balance_minor >= 0)` | `wallets` | impede saldo negativo no banco |
| Ledger append-only | repo | só `INSERT`; sem `UPDATE`/`DELETE` de lançamentos |

### Persistência

| Tabela | Papel |
| --- | --- |
| `wallets` | estado atual (`balance_minor` BIGINT + currency, version) |
| `wallet_ledger_entries` | lançamentos imutáveis (append-only) |

Migrations em `migrations/`. Repos em `internal/database` (pgx, SQL explícito). Ambiente local: `docker compose up -d` (somente Postgres nesta fase).

Aplicar migrations (exemplo):

```sh
docker compose up -d
psql "$DATABASE_URL" -f migrations/000001_wallets_ledger.up.sql
```

### Erros

- `ErrInvalidWallet` / `ErrInvalidPlayer`
- `ErrInsufficientFunds`
- `ErrInvalidMovement` / `ErrInvalidLedger`
- `ErrUninitialized`
- reutiliza `money.ErrCurrencyMismatch` quando a moeda do movimento diverge
- gateway/database: `ErrConflict`, `ErrNotFound`
- usecase: `ErrConflict`, `ErrNotFound`, `ErrInvalidInput`

## Application — use cases de carteira

Módulo `internal/usecase/wallet` — um `UseCase` com métodos (sem HTTP ainda):

| Método | Papel |
| --- | --- |
| `Open` | cria wallet (+ ledger se saldo > 0) na mesma TX |
| `Get` | leitura por id |
| `ListLedger` | ledger com cursor opaco `(created_at, id)` |
| `Reconcile` | saldo armazenado vs ΣCREDIT−ΣDEBIT; não altera saldo |

`Open` com saldo positivo ainda **não** persiste `WagerTransaction OPENING` nem outbox — `TODO(futuro)`. Wallet + ledger da abertura já vão no mesmo `Commit`.
