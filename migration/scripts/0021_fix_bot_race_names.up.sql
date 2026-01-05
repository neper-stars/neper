-- Fix bot race names: Turndrones -> Turindrones, Mcinti -> Macintis
UPDATE race SET name_plural = 'Turindrones', name_singular = 'Turindrone' WHERE id = '2' AND user_id = 'system';
UPDATE race SET name_plural = 'Macintis', name_singular = 'Macinti' WHERE id = '6' AND user_id = 'system';
