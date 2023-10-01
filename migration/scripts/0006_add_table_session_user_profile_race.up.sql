CREATE TABLE session_user_profile_race (
    session_id VARCHAR NOT NULL,
    user_profile_id VARCHAR NOT NULL,
    race_id VARCHAR NOT NULL,
    player_order integer NOT NULL,
    is_bot boolean NOT NULL DEFAULT false,
    bot_level integer,
    PRIMARY KEY (session_id, user_profile_id),
    FOREIGN KEY (user_profile_id) REFERENCES user_profile (id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES session (id) ON DELETE CASCADE,
    FOREIGN KEY (race_id) REFERENCES race (id) ON DELETE CASCADE,
    CONSTRAINT bot_level_should_be_lower_or_equal_to_4
      check (session_user_profile_race.bot_level <= 4),
    CONSTRAINT player_order_should_be_lower_or_equal_to_15
      check (session_user_profile_race.player_order <= 15)
);
