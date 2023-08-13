CREATE TABLE images
(
    id          SERIAL PRIMARY KEY,
    path        TEXT      NOT NULL,
    sha256      TEXT      NOT NULL UNIQUE,
    timestamp   TIMESTAMP NOT NULL,
    tz_location TEXT      NOT NULL,
    latitude    FLOAT     NOT NULL,
    longitude   FLOAT     NOT NULL
);

CREATE TABLE posts
(
    id        SERIAL PRIMARY KEY,
    image_id  INTEGER REFERENCES images (id),
    timestamp TIMESTAMPTZ NOT NULL
);

CREATE TABLE locations
(
    id       SERIAL PRIMARY KEY,
    image_id INTEGER REFERENCES images (id),
    city     TEXT NOT NULL,
    country  TEXT NOT NULL
);
