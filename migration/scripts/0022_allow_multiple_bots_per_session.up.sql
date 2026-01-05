-- Remove the unique constraint on (session_id, user_profile_id) to allow multiple bots per session
-- Bots use the "system" user_profile_id, so the constraint prevented adding multiple bots
-- The code now enforces that only the "system" user can have multiple entries per session
ALTER TABLE session_player_race DROP CONSTRAINT unique_player_per_session;
