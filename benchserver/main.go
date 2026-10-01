// Command benchserver starts a standalone MySQL-protocol server backed by
// go-mysql-server. It exists so the Go and Java benchmark harnesses can query
// the exact same engine process, making the comparison measure client and ORM
// overhead rather than differences between two database engines.
//
// The schema is a superset of the benchmark entities on both sides:
//
//	users            save/select/where/order/page/update/delete cases (100 rows)
//	bench_bench_users tablePrefix("bench") case, isolated from users (20 rows)
//	categories       irregular English plural coverage (20 rows)
//	orders           join association target (100 rows, uid spread over users 1..20)
//	profiles         NULL columns + enum/converter columns (100 rows)
//	delete_targets   large row pool consumed by the delete cases (20000 rows)
//
// Go model names map to these tables by the same pluralization rule the Java
// side uses (plural(prefix + "_" + toUnderline(ClassName))), so a model named
// BenchUser with prefix "bench" gives "bench_bench_users" on both sides.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/memory"
	"github.com/dolthub/go-mysql-server/server"
	gsql "github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/information_schema"
	_ "github.com/go-sql-driver/mysql"
)

const (
	addr    = "127.0.0.1:33099"
	dbName  = "anima_bench"
	seedNum = 100

	orderNum    = 100
	categoryNum = 20
	prefixNum   = 20
	deleteNum   = 20000
)

func main() {
	engine := sqle.NewDefault(gsql.NewDatabaseProvider(
		memory.NewDatabase(dbName),
		information_schema.NewInformationSchemaDatabase(),
	))
	s, err := server.NewDefaultServer(server.Config{Protocol: "tcp", Address: addr}, engine)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		if err := s.Start(); err != nil {
			log.Fatal(err)
		}
	}()

	client, err := sql.Open("mysql", fmt.Sprintf("root@tcp(%s)/%s?parseTime=true", addr, dbName))
	if err != nil {
		log.Fatal(err)
	}
	for i := 0; ; i++ {
		if err := client.Ping(); err == nil {
			break
		} else if i >= 50 {
			log.Fatalf("benchserver unreachable: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := seed(client); err != nil {
		log.Fatal(err)
	}
	log.Printf("benchserver listening on %s db=%s (%d user rows + %d orders + %d categories + %d profiles + %d prefix rows seeded)",
		addr, dbName, seedNum, orderNum, categoryNum, seedNum, prefixNum)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	client.Close()
}

// seed creates every table the benchmark harnesses touch, then fills them with
// deterministic rows so both sides scan identical data. Ids are explicit because
// DELETE does not reset AUTO_INCREMENT (MySQL semantics), which the harnesses'
// byId/where assertions depend on.
func seed(client *sql.DB) error {
	if _, err := client.Exec(`CREATE TABLE users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_name VARCHAR(255) NOT NULL DEFAULT '',
		age INT NOT NULL DEFAULT 0,
		email VARCHAR(255),
		created_at DATETIME
	)`); err != nil {
		return err
	}
	if _, err := client.Exec(`CREATE TABLE bench_bench_users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_name VARCHAR(255) NOT NULL DEFAULT '',
		age INT NOT NULL DEFAULT 0
	)`); err != nil {
		return err
	}
	if _, err := client.Exec(`CREATE TABLE categories (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(255) NOT NULL DEFAULT ''
	)`); err != nil {
		return err
	}
	if _, err := client.Exec(`CREATE TABLE orders (
		id INT AUTO_INCREMENT PRIMARY KEY,
		uid INT NOT NULL DEFAULT 0,
		product_name VARCHAR(255) NOT NULL DEFAULT '',
		create_time DATETIME
	)`); err != nil {
		return err
	}
	if _, err := client.Exec(`CREATE TABLE profiles (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_id INT NOT NULL DEFAULT 0,
		nickname VARCHAR(255),
		sex INT,
		vip INT
	)`); err != nil {
		return err
	}
	if _, err := client.Exec(`CREATE TABLE delete_targets (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(255) NOT NULL DEFAULT ''
	)`); err != nil {
		return err
	}
	for _, t := range []string{"users", "bench_bench_users", "categories", "orders", "profiles", "delete_targets"} {
		if _, err := client.Exec("DELETE FROM " + t); err != nil {
			return err
		}
	}
	for i := 1; i <= seedNum; i++ {
		if _, err := client.Exec("INSERT INTO users (id, user_name, age) VALUES (?, ?, ?)",
			i, fmt.Sprintf("user_%03d", i), i); err != nil {
			return err
		}
	}
	for i := 1; i <= prefixNum; i++ {
		if _, err := client.Exec("INSERT INTO bench_bench_users (id, user_name, age) VALUES (?, ?, ?)",
			i, fmt.Sprintf("bench_%03d", i), 20+i); err != nil {
			return err
		}
	}
	for i := 1; i <= categoryNum; i++ {
		if _, err := client.Exec("INSERT INTO categories (id, name) VALUES (?, ?)",
			i, fmt.Sprintf("cat_%03d", i)); err != nil {
			return err
		}
	}
	// uid is spread over users 1..20 so every one of them owns exactly 5
	// orders, making the hasMany join deterministic.
	for i := 1; i <= orderNum; i++ {
		uid := 1 + (i % 20)
		if _, err := client.Exec("INSERT INTO orders (id, uid, product_name, create_time) VALUES (?, ?, ?, ?)",
			i, uid, fmt.Sprintf("prod_%03d", i), "2026-01-01 00:00:00"); err != nil {
			return err
		}
	}
	// Every third profile row leaves the enum and text columns NULL: the
	// whereIsNull/hasNull read cases and the saveNull write case share it.
	for i := 1; i <= seedNum; i++ {
		var nickname any
		var sex, vip any
		if i%3 != 0 {
			nickname = fmt.Sprintf("nick_%03d", i)
			if i%2 == 0 {
				sex = 2
			} else {
				sex = 1
			}
			vip = 1 + (i % 3)
		}
		if _, err := client.Exec("INSERT INTO profiles (id, user_id, nickname, sex, vip) VALUES (?, ?, ?, ?, ?)",
			i, i, nickname, sex, vip); err != nil {
			return err
		}
	}
	// The delete pool is seeded in batched statements to keep single-statement
	// size reasonable; it only ever shrinks during a run.
	const batch = 2000
	for start := 1; start <= deleteNum; start += batch {
		end := start + batch - 1
		if end > deleteNum {
			end = deleteNum
		}
		var sb strings.Builder
		sb.WriteString("INSERT INTO delete_targets (id, name) VALUES ")
		for i := start; i <= end; i++ {
			if i > start {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "(%d, 'dt_%05d')", i, i)
		}
		if _, err := client.Exec(sb.String()); err != nil {
			return err
		}
	}
	return nil
}
