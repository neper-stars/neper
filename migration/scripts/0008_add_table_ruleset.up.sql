CREATE TABLE ruleset (
    id VARCHAR PRIMARY KEY,
    session_id VARCHAR NOT NULL,
    accelerated_bbs_play BOOLEAN NOT NULL,
    computer_players_form_alliances BOOLEAN NOT NULL,
    density INTEGER NOT NULL,
    galaxy_clumping BOOLEAN NOT NULL,
    maximum_minerals BOOLEAN NOT NULL,
    no_random_events BOOLEAN NOT NULL,
    public_player_scores BOOLEAN NOT NULL,
    slower_tech_advances BOOLEAN NOT NULL,
    starting_distance INTEGER NOT NULL,
    universe_size INTEGER NOT NULL,
    random_seed INTEGER,

    FOREIGN KEY (session_id) REFERENCES session (id) ON DELETE CASCADE,
    CONSTRAINT check_density_max
        check (ruleset.density <= 3),
    CONSTRAINT check_density_min
        check (ruleset.density >= 0),
    CONSTRAINT check_starting_distance_max
            check (ruleset.density <= 3),
    CONSTRAINT check_starting_distance_min
        check (ruleset.density >= 0),
    CONSTRAINT check_universe_size_max
        check (ruleset.density <= 4),
    CONSTRAINT check_universe_size_min
        check (ruleset.density >= 0)
);
