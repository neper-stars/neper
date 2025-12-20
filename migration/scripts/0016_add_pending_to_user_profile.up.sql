ALTER TABLE user_profile ADD COLUMN pending BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE user_profile ADD COLUMN registration_message VARCHAR;
