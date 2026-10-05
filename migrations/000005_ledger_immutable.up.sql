-- Ledger append-only enforcement (README §5.8 / §6.4).
-- Application already only INSERTs; triggers block UPDATE/DELETE at the DB.

CREATE OR REPLACE FUNCTION deny_ledger_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'wallet_ledger_entries is append-only (no UPDATE/DELETE)';
END;
$$;

CREATE TRIGGER wallet_ledger_entries_immutable
    BEFORE UPDATE OR DELETE ON wallet_ledger_entries
    FOR EACH ROW
    EXECUTE FUNCTION deny_ledger_mutation();
