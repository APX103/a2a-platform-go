package testutil

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	_ "github.com/go-sql-driver/mysql"
)

// TempMySQLDB creates an isolated MySQL database for a test and drops it on cleanup.
func TempMySQLDB(t *testing.T) *sql.DB {
	t.Helper()

	host := getenv("TEST_MYSQL_HOST", "127.0.0.1")
	port := getenv("TEST_MYSQL_PORT", "13306")
	user := getenv("TEST_MYSQL_USER", "root")
	password := getenv("TEST_MYSQL_PASSWORD", "root_secret_2024")

	adminDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/?parseTime=true&loc=Local&multiStatements=true", user, password, host, port)
	admin, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatalf("open mysql admin connection: %v", err)
	}
	if err := admin.Ping(); err != nil {
		admin.Close()
		t.Skipf("mysql test database unavailable: %v", err)
	}

	dbName := "a2a_test_" + strings.ReplaceAll(uuid.New().String(), "-", "_")
	if _, err := admin.Exec("CREATE DATABASE `" + dbName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		admin.Close()
		t.Fatalf("create mysql test database: %v", err)
	}

	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS `" + dbName + "`")
		_ = admin.Close()
	})

	dbDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local&multiStatements=true", user, password, host, port, dbName)
	db, err := sql.Open("mysql", dbDSN)
	if err != nil {
		t.Fatalf("open mysql test database: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping mysql test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
