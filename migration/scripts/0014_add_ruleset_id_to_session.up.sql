ALTER TABLE session ADD COLUMN ruleset_id VARCHAR(36) REFERENCES ruleset(id);
