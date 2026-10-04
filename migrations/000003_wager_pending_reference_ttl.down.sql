DROP INDEX IF EXISTS wager_transactions_pending_reference_due_idx;
ALTER TABLE wager_transactions DROP COLUMN IF EXISTS pending_reference_until;
