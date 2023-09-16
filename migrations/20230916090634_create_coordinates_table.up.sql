CREATE TABLE coordinates
(
    id          SERIAL PRIMARY KEY,
    latitude    FLOAT NOT NULL,
    longitude   FLOAT NOT NULL,
    location_id INTEGER REFERENCES locations (id)
);
