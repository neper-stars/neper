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
    vc_attain_tech_x_in_y_field BOOLEAN NOT NULL,
    vc_attain_tech_x_in_y_field_fields_value INTEGER,
    vc_attain_tech_x_in_y_field_tech_value INTEGER,
    vc_exceed_next_player_score_by_x BOOLEAN NOT NULL,
    vc_exceed_next_player_score_by_x_value INTEGER,
    vc_exceed_score_of_x BOOLEAN NOT NULL,
    vc_exceed_score_of_x_value INTEGER,
    vc_has_production_capacity_of_x_thousand BOOLEAN NOT NULL,
    vc_has_production_capacity_of_x_thousand_value INTEGER,
    vc_have_highest_score_after_x_years BOOLEAN NOT NULL,
    vc_have_highest_score_after_x_years_value INTEGER,
    vc_owns_x_capital_ships BOOLEAN NOT NULL,
    vc_owns_x_capital_ships_value INTEGER,
    vc_owns_x_percent_of_planets BOOLEAN NOT NULL,
    vc_owns_x_percent_of_planets_value INTEGER,
    vc_winner_must_meet_x_of_the_above INTEGER NOT NULL,
    vc_at_least_x_years_must_pass_before_a_winner_is_declared INTEGER NOT NULL,

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
        check (ruleset.density >= 0),
);
