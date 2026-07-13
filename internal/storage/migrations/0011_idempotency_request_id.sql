ALTER TABLE idempotency_keys
ADD COLUMN request_id TEXT NOT NULL DEFAULT '';
