-- Add started flag to session table
ALTER TABLE session ADD COLUMN started BOOLEAN NOT NULL DEFAULT FALSE;
