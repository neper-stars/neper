CREATE TABLE invitation (
    id VARCHAR PRIMARY KEY,
    session_id VARCHAR NOT NULL,
    user_profile_id VARCHAR NOT NULL,
    UNIQUE (session_id, user_profile_id),
    FOREIGN KEY (user_profile_id) REFERENCES user_profile (id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES session (id) ON DELETE CASCADE
);
