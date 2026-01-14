-- Add back old columns
ALTER TABLE user_profile ADD COLUMN is_active BOOLEAN;
ALTER TABLE user_profile ADD COLUMN pending BOOLEAN;

-- Migrate data back
UPDATE user_profile SET
  is_active = (state != 'inactive'),
  pending = (state = 'pending');

-- Make columns NOT NULL
ALTER TABLE user_profile ALTER COLUMN is_active SET NOT NULL;
ALTER TABLE user_profile ALTER COLUMN pending SET NOT NULL;

-- Drop new column and type
ALTER TABLE user_profile DROP COLUMN state;
DROP TYPE user_profile_state;
