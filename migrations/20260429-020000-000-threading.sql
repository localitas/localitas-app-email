ALTER TABLE emails ADD COLUMN thread_id TEXT DEFAULT '';
ALTER TABLE emails ADD COLUMN in_reply_to TEXT DEFAULT '';
ALTER TABLE emails ADD COLUMN references_header TEXT DEFAULT '';
