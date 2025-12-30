-- Remove uniqueness constraint for player per session
ALTER TABLE session_player_race
DROP CONSTRAINT unique_player_per_session;
