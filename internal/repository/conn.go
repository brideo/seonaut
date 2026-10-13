package repository

import (
	"database/sql"
	"regexp"
	"strings"
	"sync"
)

// tableNames lists every table created by the migrations. It is used to
// rewrite SQL so table names carry the configured prefix. Add new tables
// here whenever a migration creates one.
var tableNames = []string{
	"audios",
	"crawls",
	"external_links",
	"hreflangs",
	"iframes",
	"images",
	"issue_types",
	"issues",
	"links",
	"pagereports",
	"projects",
	"scripts",
	"styles",
	"users",
	"videos",
}

// tableNameRegexp matches any known table name as a whole word, so column
// names such as "project_id" or "external_links" (when matching "links")
// are left untouched.
var tableNameRegexp = regexp.MustCompile(`\b(` + strings.Join(tableNames, "|") + `)\b`)

// constraintRegexp matches named constraints. MySQL requires foreign key
// constraint names to be unique per database, so they get prefixed too.
var constraintRegexp = regexp.MustCompile("(?i)\\bCONSTRAINT\\s+`?([A-Za-z_0-9]+)`?")

// Rewriter adds a prefix to every table name (and constraint name) found in a
// SQL statement.
type Rewriter struct {
	prefix string
	cache  sync.Map
}

// NewRewriter returns a Rewriter for the given prefix. An empty prefix makes
// Rewrite a no-op.
func NewRewriter(prefix string) *Rewriter {
	return &Rewriter{prefix: prefix}
}

// Prefix returns the configured table prefix.
func (r *Rewriter) Prefix() string {
	return r.prefix
}

// Rewrite returns the query with every table name prefixed.
func (r *Rewriter) Rewrite(query string) string {
	if r.prefix == "" {
		return query
	}

	if cached, ok := r.cache.Load(query); ok {
		return cached.(string)
	}

	rewritten := tableNameRegexp.ReplaceAllString(query, r.prefix+"$1")
	rewritten = constraintRegexp.ReplaceAllString(rewritten, "CONSTRAINT `"+r.prefix+"$1`")
	r.cache.Store(query, rewritten)

	return rewritten
}

// Conn wraps *sql.DB so every statement passes through the table name
// Rewriter before reaching the database.
type Conn struct {
	*sql.DB
	rewriter *Rewriter
}

// NewConn wraps db with the given table prefix.
func NewConn(db *sql.DB, prefix string) *Conn {
	return &Conn{DB: db, rewriter: NewRewriter(prefix)}
}

func (c *Conn) Exec(query string, args ...interface{}) (sql.Result, error) {
	return c.DB.Exec(c.rewriter.Rewrite(query), args...)
}

func (c *Conn) Prepare(query string) (*sql.Stmt, error) {
	return c.DB.Prepare(c.rewriter.Rewrite(query))
}

func (c *Conn) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return c.DB.Query(c.rewriter.Rewrite(query), args...)
}

func (c *Conn) QueryRow(query string, args ...interface{}) *sql.Row {
	return c.DB.QueryRow(c.rewriter.Rewrite(query), args...)
}
