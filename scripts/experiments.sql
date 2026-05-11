-- IndexLab sample experiments
--
-- These mirror the examples in the README. Run them in psql against the
-- indexlab database (or use the lab UI to walk through them interactively).

-- 0. Setup -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users_demo (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL,
    age INT NOT NULL,
    city TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Seed a million rows the same way the API does.
TRUNCATE users_demo RESTART IDENTITY;
INSERT INTO users_demo (username, age, city, created_at)
SELECT
  'user_' || g::text,
  18 + (random() * 60)::int,
  (ARRAY['Mumbai','Delhi','Bangalore','Hyderabad','Chennai','Kolkata','Pune','Jaipur','Ahmedabad','Surat'])[1 + (random() * 9)::int],
  NOW() - (random() * INTERVAL '720 days')
FROM generate_series(1, 1000000) AS g;
ANALYZE users_demo;

-- 1. Sequential scan vs index scan --------------------------------------
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM users_demo WHERE age = 30;
CREATE INDEX idx_users_age ON users_demo(age);
ANALYZE users_demo;
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM users_demo WHERE age = 30;
DROP INDEX idx_users_age;

-- 2. Range queries ------------------------------------------------------
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM users_demo WHERE age BETWEEN 20 AND 30;
CREATE INDEX idx_users_age ON users_demo(age);
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM users_demo WHERE age BETWEEN 20 AND 30;
DROP INDEX idx_users_age;

-- 3. Composite predicate ------------------------------------------------
EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM users_demo WHERE city = 'Mumbai' AND age = 25;
CREATE INDEX idx_users_city_age ON users_demo(city, age);
ANALYZE users_demo;
EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM users_demo WHERE city = 'Mumbai' AND age = 25;
DROP INDEX idx_users_city_age;

-- 4. Index-only / covering index ---------------------------------------
EXPLAIN (ANALYZE, BUFFERS)
SELECT age, username FROM users_demo WHERE age = 27;
CREATE INDEX idx_users_age_inc_username ON users_demo(age) INCLUDE (username);
ANALYZE users_demo;
EXPLAIN (ANALYZE, BUFFERS)
SELECT age, username FROM users_demo WHERE age = 27;
-- Look for "Index Only Scan" in the plan.
DROP INDEX idx_users_age_inc_username;

-- 5. Time window query --------------------------------------------------
EXPLAIN (ANALYZE, BUFFERS)
SELECT id FROM users_demo WHERE created_at > NOW() - INTERVAL '30 days';
CREATE INDEX idx_users_created_at ON users_demo(created_at);
ANALYZE users_demo;
EXPLAIN (ANALYZE, BUFFERS)
SELECT id FROM users_demo WHERE created_at > NOW() - INTERVAL '30 days';
DROP INDEX idx_users_created_at;
