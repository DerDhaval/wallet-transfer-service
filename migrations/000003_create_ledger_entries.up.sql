-- Immutable double-entry postings. A processed transfer has exactly one debit and one credit.
CREATE TABLE ledger_entries (
    id TEXT PRIMARY KEY,
    wallet_id TEXT NOT NULL REFERENCES wallets (id),
    transfer_id TEXT NOT NULL REFERENCES transfers (id),
    entry_type TEXT NOT NULL,
    amount BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ledger_entries_id_chk CHECK (char_length(id) BETWEEN 1 AND 64),
    CONSTRAINT ledger_entries_type_chk CHECK (entry_type IN ('DEBIT', 'CREDIT')),
    CONSTRAINT ledger_entries_amount_positive CHECK (amount > 0),
    CONSTRAINT ledger_entries_transfer_type_uniq UNIQUE (transfer_id, entry_type)
);

CREATE INDEX ledger_entries_wallet_created_idx ON ledger_entries (wallet_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION validate_ledger_entry()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    t transfers%ROWTYPE;
BEGIN
    SELECT * INTO t FROM transfers WHERE id = NEW.transfer_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'ledger transfer % was not found', NEW.transfer_id;
    END IF;
    IF t.status = 'FAILED' THEN
        RAISE EXCEPTION 'cannot post ledger entries for failed transfer %', t.id;
    END IF;
    IF NEW.entry_type = 'DEBIT' AND NEW.wallet_id <> t.from_wallet_id THEN
        RAISE EXCEPTION 'debit entry wallet must be the source wallet';
    END IF;
    IF NEW.entry_type = 'CREDIT' AND NEW.wallet_id <> t.to_wallet_id THEN
        RAISE EXCEPTION 'credit entry wallet must be the destination wallet';
    END IF;
    IF NEW.amount <> t.amount THEN
        RAISE EXCEPTION 'ledger amount must match the transfer amount';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER ledger_entries_validate
BEFORE INSERT ON ledger_entries
FOR EACH ROW
EXECUTE FUNCTION validate_ledger_entry();

CREATE OR REPLACE FUNCTION reject_ledger_mutation()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    RAISE EXCEPTION 'ledger entries are immutable';
END;
$$;

CREATE TRIGGER ledger_entries_immutable
BEFORE UPDATE OR DELETE ON ledger_entries
FOR EACH ROW
EXECUTE FUNCTION reject_ledger_mutation();
