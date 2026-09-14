package transitions

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// resolveDSN returns the MYSQL_DSN environment variable, trimmed.
func resolveDSN() string {
	return strings.TrimSpace(os.Getenv("MYSQL_DSN"))
}

// newDB opens a *sql.DB from MYSQL_DSN. database/sql pools connections
// internally and dials lazily on first use, so this never blocks; it only
// panics if MYSQL_DSN itself is malformed.
func newDB() *sql.DB {
	db, err := sql.Open("mysql", resolveDSN())
	if err != nil {
		panic(fmt.Sprintf("mysql: invalid MYSQL_DSN: %v", err))
	}
	return db
}

// redactDSN strips user:password@ from a DSN before it's ever echoed back in
// a response, so credentials never leak into workflow output or logs.
func redactDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at != -1 {
		return dsn[at+1:]
	}
	return dsn
}
