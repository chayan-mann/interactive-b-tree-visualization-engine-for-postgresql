package postgreslab

import (
	"strings"
	"testing"
)

func TestValidateIdent(t *testing.T) {
	good := []string{"users_demo", "idx_users_age", "City2", "_underscore_start"}
	for _, s := range good {
		if err := validateIdent(s); err != nil {
			t.Errorf("expected %q to be valid: %v", s, err)
		}
	}
	bad := []string{"", "users; DROP TABLE", "no spaces", "weird-char", "1starts_with_digit", strings.Repeat("a", 64)}
	for _, s := range bad {
		if err := validateIdent(s); err == nil {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}

func TestGuardReadOnly(t *testing.T) {
	ok := []string{
		"SELECT * FROM users_demo",
		"  select 1;",
		"WITH cte AS (SELECT 1) SELECT * FROM cte",
		"SELECT '-- not mutating'::text",
		"SELECT * FROM users_demo WHERE city = 'DROP TABLE users_demo';",
		"SELECT 1 /* insert into users_demo */",
		"SELECT 1; -- insert attempt",
		"SELECT * FROM users_demo; \n-- comment\n",
	}
	for _, s := range ok {
		if err := guardReadOnly(s); err != nil {
			t.Errorf("expected %q to be allowed: %v", s, err)
		}
	}

	bad := []string{
		"INSERT INTO users_demo VALUES (1)",
		"DROP TABLE users_demo",
		"truncate users_demo",
		"  ALTER TABLE users_demo ADD COLUMN x INT",
		"SELECT 1; SELECT 2;",
		"WITH cte AS (UPDATE users_demo SET age = age + 1) SELECT * FROM cte",
		"/* comment */ DELETE FROM users_demo",
		`SELECT $$
CREATE TABLE hidden (id INT);
$$;`,
	}
	for _, s := range bad {
		if err := guardReadOnly(s); err == nil {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}
