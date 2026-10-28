package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
)

func migrateSQLiteUsers(destination, source string) error {
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	db, err := sqlx.Open("sqlite", destination+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var completed bool
	if err := tx.Get(&completed, "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name='wuzapi_users_db_migration')"); err != nil {
		return err
	}
	if completed {
		return nil
	}
	path, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=ro"}).String()
	if _, err := tx.Exec("ATTACH DATABASE ? AS legacy_users", uri); err != nil {
		return err
	}
	if _, err := tx.Exec("PRAGMA defer_foreign_keys=ON"); err != nil {
		return err
	}
	var objects []struct {
		Type string `db:"type"`
		Name string `db:"name"`
		SQL  string `db:"sql"`
	}
	if err := tx.Select(&objects, `SELECT type, name, sql FROM legacy_users.sqlite_master
        WHERE name NOT GLOB 'sqlite_*' AND sql IS NOT NULL
        ORDER BY CASE type WHEN 'table' THEN 0 WHEN 'index' THEN 1 ELSE 2 END, name`); err != nil {
		return err
	}
	for _, object := range objects {
		if _, err := tx.Exec(object.SQL); err != nil {
			return fmt.Errorf("create %q: %w", object.Name, err)
		}
		if object.Type == "table" {
			name := `"` + strings.ReplaceAll(object.Name, `"`, `""`) + `"`
			if _, err := tx.Exec("INSERT INTO main." + name + " SELECT * FROM legacy_users." + name); err != nil {
				return fmt.Errorf("copy %q: %w", object.Name, err)
			}
		}
	}
	var sequence bool
	if err := tx.Get(&sequence, "SELECT EXISTS(SELECT 1 FROM legacy_users.sqlite_master WHERE name='sqlite_sequence')"); err != nil {
		return err
	}
	if sequence {
		if _, err := tx.Exec(`DELETE FROM main.sqlite_sequence WHERE name IN (SELECT name FROM legacy_users.sqlite_sequence);
            INSERT INTO main.sqlite_sequence (name,seq) SELECT name,seq FROM legacy_users.sqlite_sequence`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("CREATE TABLE wuzapi_users_db_migration AS SELECT 1 AS completed"); err != nil {
		return err
	}
	return tx.Commit()
}
