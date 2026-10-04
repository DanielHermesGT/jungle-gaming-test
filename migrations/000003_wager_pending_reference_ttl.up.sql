-- TTL deadline for PENDING_REFERENCE (README §7).

ALTER TABLE wager_transactions
    ADD COLUMN pending_reference_until TIMESTAMPTZ NULL;

CREATE INDEX wager_transactions_pending_reference_due_idx
    ON wager_transactions (pending_reference_until)
    WHERE status = 'PENDING_REFERENCE';
