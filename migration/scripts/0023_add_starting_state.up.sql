-- Add 'starting' state to session_state enum
-- This state is used during game generation to prevent duplicate start commands
ALTER TYPE session_state ADD VALUE 'starting' AFTER 'pending';
