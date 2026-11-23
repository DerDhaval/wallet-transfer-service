DROP TRIGGER IF EXISTS ledger_entries_immutable ON ledger_entries;
DROP TRIGGER IF EXISTS ledger_entries_validate ON ledger_entries;
DROP FUNCTION IF EXISTS reject_ledger_mutation();
DROP FUNCTION IF EXISTS validate_ledger_entry();
DROP TABLE IF EXISTS ledger_entries;
