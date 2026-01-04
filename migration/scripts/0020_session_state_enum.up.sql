-- Fix any empty state values first (from sessions created before the fix)
UPDATE session SET state = 'pending' WHERE state = '' OR state IS NULL;

-- Create the enum type for session state
CREATE TYPE session_state AS ENUM ('pending', 'started', 'archived');

-- Convert the column from VARCHAR to the enum type
ALTER TABLE session
    ALTER COLUMN state DROP DEFAULT,
    ALTER COLUMN state TYPE session_state USING state::session_state,
    ALTER COLUMN state SET DEFAULT 'pending';
