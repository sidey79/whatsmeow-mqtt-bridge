package config

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseDefaultsToSQLite(t *testing.T) {
	cfg, err := databaseFromLookup(lookup(nil))
	if err != nil || cfg.Driver != "sqlite" || cfg.DSN != "/data/whatsapp_session.db" {
		t.Fatalf("unexpected config: %+v, %v", cfg, err)
	}
}

func TestPostgresComponentsAreEscaped(t *testing.T) {
	cfg, err := databaseFromLookup(lookup(map[string]string{
		"WA_DB_DRIVER": "postgres", "WA_DB_HOST": "db", "WA_DB_NAME": "fhem", "WA_DB_USER": "bridge",
		"WA_DB_PASSWORD": "p@ss/word", "WA_DB_SCHEMA": "whatsmeow", "WA_DB_SSLMODE": "disable",
	}))
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(cfg.DSN)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	if u.Host != "db:5432" || u.Path != "/fhem" || u.User.Username() != "bridge" || password != "p@ss/word" || u.Query().Get("search_path") != "whatsmeow" {
		t.Fatalf("unexpected DSN components: %s", cfg.DSN)
	}
}

func TestPostgresSecretFilesTakePriority(t *testing.T) {
	dir := t.TempDir()
	dsnFile := filepath.Join(dir, "dsn")
	if err := os.WriteFile(dsnFile, []byte("postgres://from-file/db\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := databaseFromLookup(lookup(map[string]string{"WA_DB_DRIVER": "postgres", "WA_DB_DSN": "postgres://from-env/db", "WA_DB_DSN_FILE": dsnFile}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DSN != "postgres://from-file/db" {
		t.Fatalf("DSN: %q", cfg.DSN)
	}
}

func TestPostgresValidation(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"driver":  {"WA_DB_DRIVER": "mysql"},
		"missing": {"WA_DB_DRIVER": "postgres"},
		"schema":  {"WA_DB_DRIVER": "postgres", "WA_DB_HOST": "db", "WA_DB_NAME": "fhem", "WA_DB_USER": "u", "WA_DB_PASSWORD": "p", "WA_DB_SCHEMA": "bad-name"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := databaseFromLookup(lookup(values)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
