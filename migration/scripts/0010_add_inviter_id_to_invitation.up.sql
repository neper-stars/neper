ALTER TABLE invitation
ADD COLUMN inviter_id VARCHAR NOT NULL DEFAULT '',
ADD CONSTRAINT invitation_inviter_id_fkey FOREIGN KEY (inviter_id) REFERENCES user_profile (id) ON DELETE CASCADE;
