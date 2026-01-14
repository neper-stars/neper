-- Create enum type for user profile state
CREATE TYPE user_profile_state AS ENUM ('pending', 'active', 'inactive');

-- Add new column
ALTER TABLE user_profile ADD COLUMN state user_profile_state;

-- Migrate existing data
UPDATE user_profile SET state =
  CASE
    WHEN is_active = false THEN 'inactive'::user_profile_state
    WHEN pending = true THEN 'pending'::user_profile_state
    ELSE 'active'::user_profile_state
  END;

-- Make column NOT NULL after migration
ALTER TABLE user_profile ALTER COLUMN state SET NOT NULL;

-- Drop old columns
ALTER TABLE user_profile DROP COLUMN is_active;
ALTER TABLE user_profile DROP COLUMN pending;
