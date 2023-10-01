CREATE TABLE user_profile_session_rel (
    user_profile_id VARCHAR NOT NULL,
    session_id VARCHAR NOT NULL,
    is_manager BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (user_profile_id, session_id),
    FOREIGN KEY (user_profile_id) REFERENCES user_profile (id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES session (id) ON DELETE CASCADE
);
