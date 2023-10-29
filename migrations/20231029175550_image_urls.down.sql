BEGIN TRANSACTION;
ALTER TABLE images
    RENAME COLUMN url_large TO path_large;
ALTER TABLE images
    RENAME COLUMN url_medium TO path_medium;
ALTER TABLE images
    RENAME COLUMN url_small TO path_small;
COMMIT;