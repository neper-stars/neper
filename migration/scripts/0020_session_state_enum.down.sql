-- Convert the column back from enum to VARCHAR
ALTER TABLE session
    ALTER COLUMN state DROP DEFAULT,
    ALTER COLUMN state TYPE VARCHAR(20) USING state::text,
    ALTER COLUMN state SET DEFAULT 'pending';

-- Drop the enum type
DROP TYPE session_state;
