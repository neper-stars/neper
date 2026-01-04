-- Drop the index
DROP INDEX IF EXISTS idx_session_state;

-- Add back the started column
ALTER TABLE session ADD COLUMN started BOOLEAN NOT NULL DEFAULT FALSE;

-- Migrate data back
UPDATE session SET started = true WHERE state = 'started';
UPDATE session SET started = false WHERE state IN ('pending', 'archived');

-- Drop the state column
ALTER TABLE session DROP COLUMN state;
