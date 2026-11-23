-- One row per idempotency key. Stores the exact response bytes for safe retries.
CREATE TABLE idempotency_records (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    transfer_id TEXT REFERENCES transfers (id),
    response_status INTEGER NOT NULL,
    response_body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT idempotency_records_key_uniq UNIQUE (idempotency_key),
    CONSTRAINT idempotency_records_key_chk CHECK (char_length(idempotency_key) BETWEEN 1 AND 255),
    CONSTRAINT idempotency_records_hash_chk CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    -- TEXT, not JSONB, so a replay returns the original bytes.
    CONSTRAINT idempotency_records_body_object_chk CHECK (left(response_body, 1) = '{'),
    -- 0 is the in-transaction claim. A deferred trigger rejects committing that placeholder.
    CONSTRAINT idempotency_records_status_chk CHECK (response_status IN (0, 201, 422))
);

CREATE INDEX idempotency_records_transfer_idx ON idempotency_records (transfer_id);

CREATE OR REPLACE FUNCTION protect_idempotency_record()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
        OR NEW.request_hash IS DISTINCT FROM OLD.request_hash
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'idempotency key and request hash are immutable';
    END IF;

    IF OLD.response_status <> 0 AND (
        NEW.response_status IS DISTINCT FROM OLD.response_status
        OR NEW.response_body IS DISTINCT FROM OLD.response_body
        OR NEW.transfer_id IS DISTINCT FROM OLD.transfer_id
    ) THEN
        RAISE EXCEPTION 'completed idempotency record cannot be changed';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER idempotency_records_protect
BEFORE UPDATE ON idempotency_records
FOR EACH ROW
EXECUTE FUNCTION protect_idempotency_record();

CREATE OR REPLACE FUNCTION assert_idempotency_complete()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    current_status INTEGER;
    current_transfer TEXT;
BEGIN
    SELECT response_status, transfer_id
    INTO current_status, current_transfer
    FROM idempotency_records
    WHERE id = NEW.id;

    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    IF current_status = 0 OR current_transfer IS NULL THEN
        RAISE EXCEPTION 'idempotency record % cannot commit before it stores a transfer and a response', NEW.id;
    END IF;
    IF current_status NOT IN (201, 422) THEN
        RAISE EXCEPTION 'idempotency record % has invalid response status %', NEW.id, current_status;
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER idempotency_records_complete
AFTER INSERT ON idempotency_records
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION assert_idempotency_complete();
