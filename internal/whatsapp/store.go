package whatsapp

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	_ "modernc.org/sqlite"
)

type StoreConfig struct {
	Driver string
	DSN    string
}

func openStore(ctx context.Context, cfg StoreConfig) (*sqlstore.Container, *store.Device, error) {
	var (
		db      *sql.DB
		dialect string
		err     error
	)
	switch cfg.Driver {
	case "sqlite":
		if err = os.MkdirAll(dir(cfg.DSN), 0700); err != nil {
			return nil, nil, err
		}
		db, err = sql.Open("sqlite", "file:"+cfg.DSN+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
		dialect = "sqlite3"
	case "postgres":
		db, err = sql.Open("pgx", cfg.DSN)
		dialect = "postgres"
	default:
		return nil, nil, fmt.Errorf("unsupported WhatsApp store driver %q", cfg.Driver)
	}
	if err != nil {
		return nil, nil, err
	}
	container := sqlstore.NewWithDB(db, dialect, nil)
	if err = container.Upgrade(ctx); err != nil {
		_ = container.Close()
		return nil, nil, err
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, nil, err
	}
	return container, device, nil
}
