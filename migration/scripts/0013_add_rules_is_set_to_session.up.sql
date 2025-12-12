-- Add rules_is_set column to session table
ALTER TABLE session ADD COLUMN rules_is_set BOOLEAN NOT NULL DEFAULT FALSE;
