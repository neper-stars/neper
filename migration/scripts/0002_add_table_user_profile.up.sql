CREATE TABLE user_profile (
    id VARCHAR PRIMARY KEY,
    email VARCHAR NOT NULL,
    is_active BOOLEAN NOT NULL,
    is_manager BOOLEAN NOT NULL DEFAULT false,
    nickname varchar NOT NULL UNIQUE,
    apikey VARCHAR
);

-- insert a system user that will be owner of the
-- bot races. It has no admin rights and not API key.
-- This user will be be usable to login. Just used as a
-- placeholder in sessions to add bots.
INSERT INTO user_profile
    (id, email, is_active, is_manager, nickname, apikey)
VALUES
    ('system', 'system@email', false, false, 'system', null)

