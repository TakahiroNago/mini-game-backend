-- Runs once when the MySQL volume is first initialised.
-- Creates a separate database for integration tests so `go test -tags=integration`
-- can truncate tables without touching development data.
CREATE DATABASE IF NOT EXISTS minigame_test
    CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
GRANT ALL PRIVILEGES ON minigame_test.* TO 'minigame'@'%';
FLUSH PRIVILEGES;
