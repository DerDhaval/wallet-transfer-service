-- Wallets hold the current balance in integer minor units.
CREATE TABLE wallets (
    id TEXT PRIMARY KEY,
    balance BIGINT NOT NULL,
    opening_balance BIGINT NOT NULL,
    currency TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT wallets_id_chk CHECK (char_length(id) BETWEEN 1 AND 64),
    CONSTRAINT wallets_balance_non_negative CHECK (balance >= 0),
    CONSTRAINT wallets_opening_balance_non_negative CHECK (opening_balance >= 0),
    CONSTRAINT wallets_currency_chk CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT wallets_status_chk CHECK (status IN ('ACTIVE', 'FROZEN', 'CLOSED'))
);

CREATE OR REPLACE FUNCTION protect_wallet_identity()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.opening_balance IS DISTINCT FROM OLD.opening_balance
        OR NEW.currency IS DISTINCT FROM OLD.currency
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'wallet identity fields are immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER wallets_protect_identity
BEFORE UPDATE ON wallets
FOR EACH ROW
EXECUTE FUNCTION protect_wallet_identity();

-- At commit, balance must equal opening_balance + credits - debits.
CREATE OR REPLACE FUNCTION assert_wallet_balance()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    current_balance BIGINT;
    opening BIGINT;
    net BIGINT;
    derived BIGINT;
BEGIN
    SELECT balance, opening_balance
    INTO current_balance, opening
    FROM wallets
    WHERE id = NEW.id;

    IF NOT FOUND THEN
        RETURN NULL;
    END IF;

    SELECT COALESCE(SUM(
        CASE
            WHEN entry_type = 'CREDIT' THEN amount
            WHEN entry_type = 'DEBIT' THEN -amount
            ELSE 0
        END
    ), 0)
    INTO net
    FROM ledger_entries
    WHERE wallet_id = NEW.id;

    derived := opening + net;
    IF derived <> current_balance THEN
        RAISE EXCEPTION 'wallet % balance % is inconsistent with ledger-derived balance %',
            NEW.id, current_balance, derived;
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER wallets_balance_matches_ledger
AFTER INSERT OR UPDATE OF balance ON wallets
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION assert_wallet_balance();
