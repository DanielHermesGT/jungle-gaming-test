-- NÃO remover estas constraints — invariantes financeiras do README §5 / §6.2 / §6.4:
--   UNIQUE (player_id, currency)
--   UNIQUE (wallet_id, transaction_id)
--   CHECK (balance_minor >= 0)
-- Ledger é append-only na aplicação (somente INSERT; sem UPDATE/DELETE de lançamentos).

CREATE TABLE wallets (
    id              TEXT        PRIMARY KEY,
    player_id       TEXT        NOT NULL,
    currency        CHAR(3)     NOT NULL,
    balance_minor   BIGINT      NOT NULL,
    version         BIGINT      NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT wallets_balance_non_negative CHECK (balance_minor >= 0),
    CONSTRAINT wallets_version_positive CHECK (version >= 1),
    CONSTRAINT wallets_player_currency_unique UNIQUE (player_id, currency)
);

CREATE TABLE wallet_ledger_entries (
    id                    TEXT        PRIMARY KEY,
    wallet_id             TEXT        NOT NULL REFERENCES wallets (id),
    transaction_id        TEXT        NOT NULL,
    direction             TEXT        NOT NULL,
    amount_minor          BIGINT      NOT NULL,
    currency              CHAR(3)     NOT NULL,
    balance_before_minor  BIGINT      NOT NULL,
    balance_after_minor   BIGINT      NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL,

    CONSTRAINT ledger_direction_valid CHECK (direction IN ('DEBIT', 'CREDIT')),
    CONSTRAINT ledger_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT ledger_balance_before_non_negative CHECK (balance_before_minor >= 0),
    CONSTRAINT ledger_balance_after_non_negative CHECK (balance_after_minor >= 0),
    CONSTRAINT ledger_wallet_transaction_unique UNIQUE (wallet_id, transaction_id)
);

CREATE INDEX wallet_ledger_entries_wallet_created_idx
    ON wallet_ledger_entries (wallet_id, created_at, id);
