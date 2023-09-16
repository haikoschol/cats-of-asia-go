ALTER TABLE locations
    ADD COLUMN image_id INTEGER REFERENCES images (id);
