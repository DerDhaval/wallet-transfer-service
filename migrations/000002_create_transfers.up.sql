-- Transfers start as PENDING and must commit as PROCESSED or FAILED.
CREATE TABLE transfers (
    id TEXT PRIMARY KEY,
    from_wallet_id TEXT NOT NULL REFERENCES wallets (id),
    to_wallet_id TEXT NOT NULL REFERENCES wallets (id),
    amount BIGINT NOT NULL,
    currency TEXT NOT NULL,
    status TEXT NOT NULL,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT transfers_id_chk CHECK (char_length(id) BETWEEN 1 AND 64),
    CONSTRAINT transfers_amount_positive CHECK (amount > 0),
    CONSTRAINT transfers_distinct_wallets CHECK (from_wallet_id <> to_wallet_id),
    CONSTRAINT transfers_currency_chk CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT transfers_status_chk CHECK (status IN ('PENDING', 'PROCESSED', 'FAILED')),
    CONSTRAINT transfers_failure_reason_chk CHECK (
        (status = 'FAILED' AND failure_reason IS NOT NULL AND char_length(failure_reason) > 0)
        OR (status <> 'FAILED' AND failure_reason IS NULL)
    )
);

CREATE INDEX transfers_from_wallet_idx ON transfers (from_wallet_id);
CREATE INDEX transfers_to_wallet_idx ON transfers (to_wallet_id);
CREATE INDEX transfers_created_idx ON transfers (created_at DESC);

CREATE OR REPLACE FUNCTION transfers_must_start_pending()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF NEW.status <> 'PENDING' THEN
        RAISE EXCEPTION 'transfers must be created in PENDING status';
    END IF;
    IF NEW.failure_reason IS NOT NULL THEN
        RAISE EXCEPTION 'pending transfer cannot have a failure reason';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER transfers_insert_pending
BEFORE INSERT ON transfers
FOR EACH ROW
EXECUTE FUNCTION transfers_must_start_pending();

CREATE OR REPLACE FUNCTION enforce_transfer_transition()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.from_wallet_id IS DISTINCT FROM OLD.from_wallet_id
        OR NEW.to_wallet_id IS DISTINCT FROM OLD.to_wallet_id
        OR NEW.amount IS DISTINCT FROM OLD.amount
        OR NEW.currency IS DISTINCT FROM OLD.currency
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'transfer financial fields are immutable';
    END IF;

    IF OLD.status = 'PENDING' THEN
        IF NEW.status NOT IN ('PENDING', 'PROCESSED', 'FAILED') THEN
            RAISE EXCEPTION 'invalid transfer status transition from % to %', OLD.status, NEW.status;
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status
        OR NEW.failure_reason IS DISTINCT FROM OLD.failure_reason
    THEN
        RAISE EXCEPTION 'terminal transfer status cannot change';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER transfers_enforce_transition
BEFORE UPDATE ON transfers
FOR EACH ROW
EXECUTE FUNCTION enforce_transfer_transition();

-- Re-read at commit so PENDING cannot survive outside the transaction.
CREATE OR REPLACE FUNCTION assert_transfer_committed()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    t transfers%ROWTYPE;
    debit_count INTEGER;
    credit_count INTEGER;
    debit_amount BIGINT;
    credit_amount BIGINT;
    entry_count INTEGER;
    source_currency TEXT;
    destination_currency TEXT;
    source_status TEXT;
    destination_status TEXT;
BEGIN
    SELECT * INTO t FROM transfers WHERE id = NEW.id;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;

    IF t.status = 'PENDING' THEN
        RAISE EXCEPTION 'transfer % cannot commit while PENDING', t.id;
    END IF;

    SELECT
        COUNT(*) FILTER (WHERE entry_type = 'DEBIT'),
        COUNT(*) FILTER (WHERE entry_type = 'CREDIT'),
        COALESCE(SUM(amount) FILTER (WHERE entry_type = 'DEBIT'), 0),
        COALESCE(SUM(amount) FILTER (WHERE entry_type = 'CREDIT'), 0),
        COUNT(*)
    INTO debit_count, credit_count, debit_amount, credit_amount, entry_count
    FROM ledger_entries
    WHERE transfer_id = t.id;

    IF t.status = 'PROCESSED' THEN
        SELECT currency, status INTO source_currency, source_status
        FROM wallets WHERE id = t.from_wallet_id;
        SELECT currency, status INTO destination_currency, destination_status
        FROM wallets WHERE id = t.to_wallet_id;

        IF source_currency IS DISTINCT FROM destination_currency
            OR source_currency IS DISTINCT FROM t.currency
        THEN
            RAISE EXCEPTION 'transfer % currency does not match the wallets', t.id;
        END IF;
        IF source_status <> 'ACTIVE' OR destination_status <> 'ACTIVE' THEN
            RAISE EXCEPTION 'processed transfer % requires active wallets', t.id;
        END IF;
        IF debit_count <> 1 OR credit_count <> 1 OR entry_count <> 2
            OR debit_amount <> t.amount OR credit_amount <> t.amount
        THEN
            RAISE EXCEPTION 'processed transfer % must have exactly one matching debit and credit', t.id;
        END IF;
    ELSIF t.status = 'FAILED' THEN
        IF entry_count <> 0 THEN
            RAISE EXCEPTION 'failed transfer % must not have ledger entries', t.id;
        END IF;
    ELSE
        RAISE EXCEPTION 'transfer % has unknown status %', t.id, t.status;
    END IF;

    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER transfers_assert_committed_on_insert
AFTER INSERT ON transfers
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION assert_transfer_committed();

CREATE CONSTRAINT TRIGGER transfers_assert_committed_on_update
AFTER UPDATE ON transfers
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION assert_transfer_committed();
