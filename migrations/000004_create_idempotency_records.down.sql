DROP TRIGGER IF EXISTS idempotency_records_complete ON idempotency_records;
DROP TRIGGER IF EXISTS idempotency_records_protect ON idempotency_records;
DROP FUNCTION IF EXISTS assert_idempotency_complete();
DROP FUNCTION IF EXISTS protect_idempotency_record();
DROP TABLE IF EXISTS idempotency_records;
