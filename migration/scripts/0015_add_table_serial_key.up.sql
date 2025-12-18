CREATE TABLE serial_key (
    key VARCHAR(8) PRIMARY KEY,
    user_profile_id VARCHAR REFERENCES user_profile(id) ON DELETE SET NULL,
    assigned_at TIMESTAMP
);

-- Index for finding available keys quickly
CREATE INDEX idx_serial_key_available ON serial_key(key) WHERE user_profile_id IS NULL;

-- Add serial_key column to user_profile
ALTER TABLE user_profile ADD COLUMN serial_key VARCHAR(8) REFERENCES serial_key(key);
