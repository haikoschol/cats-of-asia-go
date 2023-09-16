ALTER TABLE coordinates ADD CONSTRAINT latitude_and_longitude UNIQUE (latitude, longitude);
