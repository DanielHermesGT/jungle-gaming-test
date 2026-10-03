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
| Persistência (planejada) | `BIGINT` (minor) + `CHAR(3)`/`TEXT` (currency) |

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
- Saldo de carteira não-negativo é responsabilidade do agregado `Wallet` (e, depois, das constraints do banco).

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
- `Rehydrate` reconstrói o estado persistido sem reaplicar movimentos nem gerar ledger.
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

### Concorrência (planejada na persistência)

No domínio, a versão acompanha mudanças de saldo. Entre processos, a aplicação usará:

1. `SELECT … FOR UPDATE` na linha da carteira dentro da TX SQL (lock por carteira, nunca global)
2. persistência atômica de saldo + ledger (+ demais registros da operação)
3. constraints no banco: unicidade `(player_id, currency)`, unicidade `(wallet_id, transaction_id)`, saldo ≥ 0, ledger append-only (sem update/delete)

Assim evitamos lost updates e saldo negativo mesmo com várias instâncias.

### Persistência (planejada)

| Tabela | Papel |
| --- | --- |
| `wallets` | estado atual (saldo em `BIGINT` minor + currency, version) |
| `wallet_ledger_entries` | lançamentos imutáveis |

### Erros

- `ErrInvalidWallet` / `ErrInvalidPlayer`
- `ErrInsufficientFunds`
- `ErrInvalidMovement` / `ErrInvalidLedger`
- `ErrUninitialized`
- reutiliza `money.ErrCurrencyMismatch` quando a moeda do movimento diverge
