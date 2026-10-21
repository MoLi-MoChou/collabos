package migrations

import (
	"errors"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func open(dbURL string) (*migrate.Migrate, error) {
	src, err := iofs.New(FS, ".")
	if err != nil {
		return nil, err
	}
	u := dbURL
	for _, p := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(u, p) {
			u = "pgx5://" + strings.TrimPrefix(u, p)
		}
	}
	return migrate.NewWithSourceInstance("iofs", src, u)
}

// Up applies all pending migrations.
func Up(dbURL string) error {
	m, err := open(dbURL)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// Down reverts all migrations.
func Down(dbURL string) error {
	m, err := open(dbURL)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// Version reports the current version.
func Version(dbURL string) (uint, bool, error) {
	m, err := open(dbURL)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()
	return m.Version()
}
