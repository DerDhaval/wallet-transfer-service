DROP TRIGGER IF EXISTS wallets_balance_matches_ledger ON wallets;
DROP TRIGGER IF EXISTS wallets_protect_identity ON wallets;
DROP FUNCTION IF EXISTS assert_wallet_balance();
DROP FUNCTION IF EXISTS protect_wallet_identity();
DROP TABLE IF EXISTS wallets;
