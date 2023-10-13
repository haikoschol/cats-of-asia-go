CREATE TABLE locations
(
    id       SERIAL PRIMARY KEY,
    city     TEXT NOT NULL,
    country  TEXT NOT NULL,
    timezone TEXT NOT NULL,
    UNIQUE (city, country)
);

CREATE TABLE coordinates
(
    id          SERIAL PRIMARY KEY,
    latitude    FLOAT NOT NULL,
    longitude   FLOAT NOT NULL,
    location_id INTEGER REFERENCES locations (id),
    UNIQUE (latitude, longitude)
);

CREATE TABLE images
(
    id            SERIAL PRIMARY KEY,
    path_small    TEXT      NOT NULL,
    path_medium   TEXT      NOT NULL,
    path_large    TEXT      NOT NULL,
    sha256        TEXT      NOT NULL UNIQUE,
    timestamp     TIMESTAMP NOT NULL,
    coordinate_id INTEGER REFERENCES coordinates (id)
);

CREATE TABLE platforms
(
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    profile_url TEXT NOT NULL UNIQUE
);

INSERT INTO platforms (name, profile_url)
VALUES ('Mastodon', 'https://botsin.space/@CatsOfAsia'),
       ('X', 'https://twitter.com/CatsOfAsia');

CREATE TABLE posts
(
    id          SERIAL PRIMARY KEY,
    image_id    INTEGER REFERENCES images (id),
    platform_id INTEGER REFERENCES platforms (id),
    timestamp   TIMESTAMPTZ DEFAULT now()
);
