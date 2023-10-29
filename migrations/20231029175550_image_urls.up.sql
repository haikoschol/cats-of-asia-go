BEGIN TRANSACTION;
ALTER TABLE images
    RENAME COLUMN path_large TO url_large;
ALTER TABLE images
    RENAME COLUMN path_medium TO url_medium;
ALTER TABLE images
    RENAME COLUMN path_small TO url_small;
COMMIT;