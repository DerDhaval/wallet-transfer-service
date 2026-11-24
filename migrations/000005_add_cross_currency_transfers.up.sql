-- Cross-currency transfers.
-- A transfer debits `amount` in `currency` and credits `to_amount` in `to_currency`.
-- `exchange_rate` is the rate used (1 currency = exchange_rate to_currency), kept for audit.
-- Same-currency transfers keep to_amount = amount and exchange_rate = '1'.
-- to_amount and exchange_rate stay NULL only when a FAILED attempt never reached a valid conversion
-- (for example the wallet is not active, or the amount is too small to convert).
ALTER TABLE transfers
    ADD COLUMN to_currency TEXT,
    ADD COLUMN to_amount BIGINT,
    ADD COLUMN exchange_rate TEXT;

-- Backfill existing rows. The commit-time triggers are switched off for this one statement
-- (they would otherwise queue events that block the ALTER TABLE below); every existing
-- transfer was same-currency, so the copied values are already consistent.
ALTER TABLE transfers DISABLE TRIGGER USER;
UPDATE transfers
SET to_currency = currency, to_amount = amount, exchange_rate = '1';
ALTER TABLE transfers ENABLE TRIGGER USER;

ALTER TABLE transfers
    ALTER COLUMN to_currency SET NOT NULL,
    ADD CONSTRAINT transfers_to_currency_chk CHECK (to_currency ~ '^[A-Z]{3}$'),
    ADD CONSTRAINT transfers_to_amount_positive CHECK (to_amount IS NULL OR to_amount > 0),
    -- Positive decimal text, e.g. 1, 0.0113, 88.5
    ADD CONSTRAINT transfers_exchange_rate_chk CHECK (
        exchange_rate IS NULL OR exchange_rate ~ '^(0\.[0-9]*[1-9][0-9]*|[1-9][0-9]*(\.[0-9]+)?)$'
    ),
    ADD CONSTRAINT transfers_same_currency_rate_chk CHECK (
        currency <> to_currency
        OR ((to_amount IS NULL OR to_amount = amount) AND (exchange_rate IS NULL OR exchange_rate = '1'))
    ),
    ADD CONSTRAINT transfers_processed_conversion_chk CHECK (
        status <> 'PROCESSED' OR (to_amount IS NOT NULL AND exchange_rate IS NOT NULL)
    );

-- Financial fields, including the conversion, are immutable once the transfer exists.
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
        OR NEW.to_amount IS DISTINCT FROM OLD.to_amount
        OR NEW.to_currency IS DISTINCT FROM OLD.to_currency
        OR NEW.exchange_rate IS DISTINCT FROM OLD.exchange_rate
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

-- The debit is `amount`, the credit is `to_amount`. Each wallet keeps its own currency.
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
    IF NEW.entry_type = 'DEBIT' AND NEW.amount <> t.amount THEN
        RAISE EXCEPTION 'ledger debit must match the transfer amount';
    END IF;
    IF NEW.entry_type = 'CREDIT' AND NEW.amount <> t.to_amount THEN
        RAISE EXCEPTION 'ledger credit must match the converted transfer amount';
    END IF;
    RETURN NEW;
END;
$$;

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

        IF source_currency IS DISTINCT FROM t.currency
            OR destination_currency IS DISTINCT FROM t.to_currency
        THEN
            RAISE EXCEPTION 'transfer % currencies do not match the wallets', t.id;
        END IF;
        IF source_status <> 'ACTIVE' OR destination_status <> 'ACTIVE' THEN
            RAISE EXCEPTION 'processed transfer % requires active wallets', t.id;
        END IF;
        IF debit_count <> 1 OR credit_count <> 1 OR entry_count <> 2
            OR debit_amount <> t.amount OR credit_amount <> t.to_amount
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
