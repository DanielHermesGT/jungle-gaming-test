-- WagerTransaction (README §6.3 / §7): INTERNAL OPENING vs EXTERNAL provider ops.
-- amount_minor + currency align with wallets/ledger (no float).

CREATE TABLE wager_transactions (
    id                                  TEXT        PRIMARY KEY,
    origin                              TEXT        NOT NULL,
    kind                                TEXT        NOT NULL,
    status                              TEXT        NOT NULL,
    wallet_id                           TEXT        NOT NULL REFERENCES wallets (id),
    player_id                           TEXT        NOT NULL,
    amount_minor                        BIGINT      NOT NULL,
    currency                            CHAR(3)     NOT NULL,
    created_at                          TIMESTAMPTZ NOT NULL,
    updated_at                          TIMESTAMPTZ NOT NULL,
    failure_code                        TEXT        NULL,
    result_balance_minor                BIGINT      NULL,

    -- EXTERNAL-only (NULL for INTERNAL)
    provider_id                         TEXT        NULL,
    external_transaction_id             TEXT        NULL,
    idempotency_key                     TEXT        NULL,
    payload_hash                        TEXT        NULL,
    round_id                            TEXT        NULL,
    game_id                             TEXT        NULL,
    reference_external_transaction_id   TEXT        NULL,
    resolved_reference_id               TEXT        NULL,

    CONSTRAINT wager_origin_valid CHECK (origin IN ('INTERNAL', 'EXTERNAL')),
    CONSTRAINT wager_kind_valid CHECK (kind IN ('OPENING', 'BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK')),
    CONSTRAINT wager_status_valid CHECK (status IN (
        'PENDING', 'PENDING_REFERENCE', 'PROCESSED', 'REJECTED', 'FAILED'
    )),

    CONSTRAINT wager_internal_opening CHECK (
        (origin = 'INTERNAL' AND kind = 'OPENING'
            AND provider_id IS NULL
            AND external_transaction_id IS NULL
            AND idempotency_key IS NULL
            AND payload_hash IS NULL
            AND round_id IS NULL
            AND game_id IS NULL
            AND reference_external_transaction_id IS NULL)
        OR
        (origin = 'EXTERNAL' AND kind <> 'OPENING'
            AND provider_id IS NOT NULL
            AND external_transaction_id IS NOT NULL
            AND idempotency_key IS NOT NULL
            AND payload_hash IS NOT NULL
            AND round_id IS NOT NULL
            AND game_id IS NOT NULL)
    ),

    CONSTRAINT wager_amount_by_kind CHECK (
        (kind = 'LOSS' AND amount_minor = 0)
        OR (kind IN ('BET', 'WIN', 'REFUND', 'ROLLBACK') AND amount_minor > 0)
        OR (kind = 'OPENING' AND amount_minor >= 0)
    )
);

-- One OPENING credit per wallet (README §9).
CREATE UNIQUE INDEX wager_transactions_opening_wallet_uidx
    ON wager_transactions (wallet_id)
    WHERE kind = 'OPENING';

CREATE UNIQUE INDEX wager_transactions_provider_external_uidx
    ON wager_transactions (provider_id, external_transaction_id)
    WHERE origin = 'EXTERNAL';

CREATE UNIQUE INDEX wager_transactions_idempotency_uidx
    ON wager_transactions (idempotency_key)
    WHERE origin = 'EXTERNAL';

CREATE INDEX wager_transactions_status_idx
    ON wager_transactions (status);

CREATE INDEX wager_transactions_provider_external_idx
    ON wager_transactions (provider_id, external_transaction_id);
