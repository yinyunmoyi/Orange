package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"aaa_word/biz/config"
	"aaa_word/biz/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("database migration completed")
}

func run() error {
	dsn := config.Get("MYSQL_DSN", "")
	if strings.TrimSpace(dsn) == "" {
		return errors.New("MYSQL_DSN is not configured")
	}
	connection, err := db.Open(dsn)
	if err != nil {
		return err
	}
	defer db.CloseConnection(connection)

	if err := db.AutoMigrate(connection); err != nil {
		return errors.New("apply database schema failed")
	}
	return nil
}
