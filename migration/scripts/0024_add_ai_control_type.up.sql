CREATE TYPE ai_expert_type AS ENUM ('HE', 'SS', 'IS', 'CA', 'PP', 'AR');

ALTER TABLE session_player_race ADD COLUMN ai_control_type ai_expert_type NULL;
