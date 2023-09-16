ALTER TABLE locations ADD CONSTRAINT city_and_country UNIQUE (city, country);
