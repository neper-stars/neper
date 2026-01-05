-- Restore the unique constraint on (session_id, user_profile_id)
-- Note: This will fail if there are multiple bots in any session
ALTER TABLE session_player_race ADD CONSTRAINT unique_player_per_session UNIQUE (session_id, user_profile_id);
