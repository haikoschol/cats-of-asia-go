ALTER TABLE posts ADD CONSTRAINT posts_unique_image_platform UNIQUE (image_id, platform_id);
