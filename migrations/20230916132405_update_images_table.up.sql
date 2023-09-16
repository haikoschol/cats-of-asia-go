ALTER TABLE images
    DROP COLUMN latitude,
    DROP COLUMN longitude,
    DROP COLUMN tz_location,
    ADD COLUMN coordinate_id INTEGER REFERENCES coordinates (id);
