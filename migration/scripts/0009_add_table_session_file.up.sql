CREATE TABLE session_file
(
    id VARCHAR PRIMARY KEY,
    session_id VARCHAR NOT NULL,
    year INTEGER NOT NULL,
    universe varchar NOT NULL,
    hostfile varchar NOT NULL,
    turns JSONB NOT NULL DEFAULT '[]'::JSONB,
    orders JSONB NOT NULL DEFAULT '[]'::JSONB,

    FOREIGN KEY (session_id) REFERENCES session (id) ON DELETE CASCADE,
    CONSTRAINT check_year_min
        check (session_file.year >= 2401)
);
