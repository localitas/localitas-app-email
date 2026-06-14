CREATE TABLE IF NOT EXISTS drafts (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL DEFAULT '',
    to_addresses TEXT NOT NULL DEFAULT '',
    cc_addresses TEXT NOT NULL DEFAULT '',
    bcc_addresses TEXT NOT NULL DEFAULT '',
    subject TEXT NOT NULL DEFAULT '',
    body_text TEXT NOT NULL DEFAULT '',
    body_html TEXT NOT NULL DEFAULT '',
    in_reply_to TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);
