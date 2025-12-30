-- Add uniqueness constraint to prevent a player from being added twice to the same session
ALTER TABLE session_player_race
ADD CONSTRAINT unique_player_per_session UNIQUE (session_id, user_profile_id);
