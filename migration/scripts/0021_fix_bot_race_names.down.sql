-- Revert bot race names: Turindrones -> Turndrones, Macintis -> Mcinti
UPDATE race SET name_plural = 'Turndrones', name_singular = 'Turndrone' WHERE id = '2' AND user_id = 'system';
UPDATE race SET name_plural = 'Mcinti', name_singular = 'Mcinte' WHERE id = '6' AND user_id = 'system';
