-- Add state column with default 'pending'
ALTER TABLE session ADD COLUMN state VARCHAR(20) NOT NULL DEFAULT 'pending';

-- Migrate existing data
UPDATE session SET state = 'started' WHERE started = true;
UPDATE session SET state = 'pending' WHERE started = false;

-- Drop the old started column
ALTER TABLE session DROP COLUMN started;

-- Add index on state column for faster filtering
CREATE INDEX idx_session_state ON session(state);
