ALTER TABLE media_items
ADD COLUMN runtime_minutes INTEGER CHECK (runtime_minutes IS NULL OR runtime_minutes >= 0);
