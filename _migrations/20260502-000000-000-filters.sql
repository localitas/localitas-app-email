CREATE TABLE IF NOT EXISTS filters (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    rule TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
