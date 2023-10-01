CREATE TABLE race (
    id VARCHAR PRIMARY KEY,
    name_plural VARCHAR NOT NULL,
    name_singular VARCHAR NOT NULL,
    user_id VARCHAR NOT NULL,
    data VARCHAR NOT NULL,
    FOREIGN KEY (user_id) REFERENCES user_profile (id) ON DELETE CASCADE
);

-- those a re default races for bots. They are owned by a "system" user
-- and will be used only by session admins to add bots to their games
INSERT INTO race (id, name_plural, name_singular, user_id, data)
VALUES
    ('0', 'random', 'random', 'system', 'no data for a bot race'),
    ('1', 'Robotoids', 'Robotoid', 'system', 'no data for a bot race'),
    ('2', 'Turndrones', 'Turndrone', 'system', 'no data for a bot race'),
    ('3', 'Automitrons', 'Automitron', 'system', 'no data for a bot race'),
    ('4', 'Rototills', 'Rototill', 'system', 'no data for a bot race'),
    ('5', 'Cybertrons', 'Cybertron', 'system', 'no data for a bot race'),
    ('6', 'Mcinti', 'Mcinte', 'system', 'no data for a bot race');

