-- Add started flag to session table
ALTER TABLE session_player_race ADD COLUMN ready BOOLEAN NOT NULL DEFAULT FALSE;
