package anima

import (
	"fmt"
	"net"
	"os"
	"testing"

	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/memory"
	"github.com/dolthub/go-mysql-server/server"
	gsql "github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/information_schema"
	_ "github.com/go-sql-driver/mysql"
)

const (
	testDB string = "anima_test"
)

// testPort is resolved by TestMain from the server's ephemeral bind; tests
// that build their own DSN read it instead of a fixed constant.
var testPort int

// TestMain starts an in-process MySQL-protocol server (carried over from the anima tests
// infrastructure), creates the users table, and registers the global instance via anima.Open.
func TestMain(m *testing.M) {
	engine := sqle.NewDefault(gsql.NewDatabaseProvider(
		memory.NewDatabase(testDB),
		information_schema.NewInformationSchemaDatabase(),
	))
	// Port 0 lets the OS pick a free port: two concurrent `go test` runs of
	// this package (multi-session development) or a lingering stale server
	// previously collided on the fixed testPort and panicked with
	// "bind: address already in use" before any test ran.
	config := server.Config{
		Protocol: "tcp",
		Address:  "127.0.0.1:0",
	}
	s, err := server.NewDefaultServer(config, engine)
	if err != nil {
		panic(err)
	}
	testPort, err = portOf(s.Listener.Addr())
	if err != nil {
		panic(err)
	}
	go s.Start()

	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%d)/%s?parseTime=true", testPort, testDB)
	a, err := Open(dsn)
	if err != nil {
		panic(err)
	}
	a.EnableSQLStatistic(false) // Silenced during tests; the statistic output path has its own dedicated case
	for _, ddl := range []string{
		`CREATE TABLE users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			user_name VARCHAR(255) NOT NULL DEFAULT '',
			age INT NOT NULL DEFAULT 0,
			email VARCHAR(255),
			created_at DATETIME
		)`,
		`CREATE TABLE products (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255) NOT NULL DEFAULT '',
			stock INT NULL,
			color INT NOT NULL DEFAULT 0,
			price BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE teams (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255) NOT NULL DEFAULT '',
			captain_id INT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE players (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255) NOT NULL DEFAULT '',
			team_id INT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE tiny_scores (
			id INT AUTO_INCREMENT PRIMARY KEY,
			val TINYINT NOT NULL DEFAULT 0,
			big SMALLINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE posts (
			id INT AUTO_INCREMENT PRIMARY KEY,
			title VARCHAR(255) NOT NULL DEFAULT '',
			state VARCHAR(16) NOT NULL DEFAULT 'draft'
		)`,
		`CREATE TABLE name_only_rows (
			name VARCHAR(255) NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE t_users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			user_name VARCHAR(255) NOT NULL DEFAULT '',
			age INT NOT NULL DEFAULT 0,
			email VARCHAR(255),
			created_at DATETIME
		)`,
	} {
		if _, err := a.db.Exec(ddl); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	s.Close()    // release the listen socket before exiting, or a quickly repeated
	a.db.Close() // test run fails to rebind the port while the old process lingers
	os.Exit(code)
}

// portOf extracts the ephemeral bind port from the test server's listener.
func portOf(addr net.Addr) (int, error) {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("test server listener addr is %T, want *net.TCPAddr", addr)
	}
	return tcp.Port, nil
}
