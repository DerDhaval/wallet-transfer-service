DROP TRIGGER IF EXISTS transfers_assert_committed_on_update ON transfers;
DROP TRIGGER IF EXISTS transfers_assert_committed_on_insert ON transfers;
DROP TRIGGER IF EXISTS transfers_enforce_transition ON transfers;
DROP TRIGGER IF EXISTS transfers_insert_pending ON transfers;
DROP FUNCTION IF EXISTS assert_transfer_committed();
DROP FUNCTION IF EXISTS enforce_transfer_transition();
DROP FUNCTION IF EXISTS transfers_must_start_pending();
DROP TABLE IF EXISTS transfers;
