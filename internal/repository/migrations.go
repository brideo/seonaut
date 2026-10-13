package repository

import (
	"bytes"
	"database/sql"
	"errors"
	"io"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/file"
)

// MigrationsTable is the name, before prefixing, of the table golang-migrate
// uses to track applied migrations.
const MigrationsTable = "schema_migrations"

// RunMigrations applies all pending migrations found in dir, rewriting table
// names with the given prefix as they are read.
func RunMigrations(db *sql.DB, dir, prefix string) error {
	driver, err := mysql.WithInstance(db, &mysql.Config{
		MigrationsTable: prefix + MigrationsTable,
	})
	if err != nil {
		return err
	}

	src, err := (&file.File{}).Open("file://" + dir)
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("prefixed-file", &prefixedSource{
		Driver:   src,
		rewriter: NewRewriter(prefix),
	}, "mysql", driver)
	if err != nil {
		return err
	}

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

// prefixedSource wraps a migrate source.Driver and rewrites the SQL of each
// migration so table names carry the configured prefix.
type prefixedSource struct {
	source.Driver
	rewriter *Rewriter
}

func (s *prefixedSource) ReadUp(version uint) (io.ReadCloser, string, error) {
	r, id, err := s.Driver.ReadUp(version)
	return s.rewrite(r), id, err
}

func (s *prefixedSource) ReadDown(version uint) (io.ReadCloser, string, error) {
	r, id, err := s.Driver.ReadDown(version)
	return s.rewrite(r), id, err
}

func (s *prefixedSource) rewrite(r io.ReadCloser) io.ReadCloser {
	if r == nil {
		return nil
	}
	defer r.Close()

	b, err := io.ReadAll(r)
	if err != nil {
		return io.NopCloser(bytes.NewReader(nil))
	}

	return io.NopCloser(bytes.NewReader([]byte(s.rewriter.Rewrite(string(b)))))
}
