package config

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestPostgresPasswordFileOverridesEnvironment(t *testing.T) {
	t.Parallel()

	secretPath := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(secretPath, []byte("file secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	values := map[string]string{
		"WA_DB_DRIVER":        "postgres",
		"WA_DB_HOST":          "db",
		"WA_DB_NAME":          "bridge",
		"WA_DB_USER":          "bridge",
		"WA_DB_PASSWORD":      "environment secret",
		"WA_DB_PASSWORD_FILE": secretPath,
	}

	cfg, err := databaseFromLookup(lookup(values))
	if err != nil {
		t.Fatal(err)
	}

	dsn, err := url.Parse(cfg.DSN)
	if err != nil {
		t.Fatal(err)
	}
	password, ok := dsn.User.Password()
	if !ok || password != "file secret" {
		t.Fatalf("password = %q, %v; want file secret, true", password, ok)
	}
}
