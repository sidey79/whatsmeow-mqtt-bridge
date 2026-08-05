package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type DatabaseConfig struct {
	Driver string
	DSN    string
}

var postgresIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func databaseFromLookup(lookup func(string) (string, bool)) (DatabaseConfig, error) {
	get := func(key, fallback string) string {
		if value, ok := lookup(key); ok {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	driver := strings.ToLower(get("WA_DB_DRIVER", "sqlite"))
	switch driver {
	case "sqlite":
		path := get("WA_DB_PATH", "/data/whatsapp_session.db")
		if path == "" {
			return DatabaseConfig{}, fmt.Errorf("WA_DB_PATH must not be empty")
		}
		return DatabaseConfig{Driver: driver, DSN: path}, nil
	case "postgres":
		return postgresConfig(lookup, get)
	default:
		return DatabaseConfig{}, fmt.Errorf("WA_DB_DRIVER must be sqlite or postgres")
	}
}

func postgresConfig(lookup func(string) (string, bool), get func(string, string) string) (DatabaseConfig, error) {
	if path := get("WA_DB_DSN_FILE", ""); path != "" {
		dsn, err := readSecret(path)
		if err != nil {
			return DatabaseConfig{}, fmt.Errorf("read WA_DB_DSN_FILE: %w", err)
		}
		return DatabaseConfig{Driver: "postgres", DSN: dsn}, nil
	}
	if dsn := get("WA_DB_DSN", ""); dsn != "" {
		return DatabaseConfig{Driver: "postgres", DSN: dsn}, nil
	}
	host, port := get("WA_DB_HOST", ""), get("WA_DB_PORT", "5432")
	name, user := get("WA_DB_NAME", ""), get("WA_DB_USER", "")
	password := get("WA_DB_PASSWORD", "")
	if path := get("WA_DB_PASSWORD_FILE", ""); path != "" {
		var err error
		password, err = readSecret(path)
		if err != nil {
			return DatabaseConfig{}, fmt.Errorf("read WA_DB_PASSWORD_FILE: %w", err)
		}
	}
	if host == "" || name == "" || user == "" || password == "" {
		return DatabaseConfig{}, fmt.Errorf("WA_DB_HOST, WA_DB_NAME, WA_DB_USER and a password are required for postgres")
	}
	u := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port), Path: "/" + name, User: url.UserPassword(user, password)}
	query := u.Query()
	query.Set("sslmode", get("WA_DB_SSLMODE", "require"))
	if schema := get("WA_DB_SCHEMA", ""); schema != "" {
		if !postgresIdentifier.MatchString(schema) {
			return DatabaseConfig{}, fmt.Errorf("WA_DB_SCHEMA is invalid")
		}
		query.Set("search_path", `"`+schema+`"`)
	}
	u.RawQuery = query.Encode()
	return DatabaseConfig{Driver: "postgres", DSN: u.String()}, nil
}

func readSecret(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	secret := strings.TrimSpace(string(content))
	if secret == "" {
		return "", fmt.Errorf("secret file is empty")
	}
	return secret, nil
}
