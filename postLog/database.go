package postLog

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var (
	logsDB    *sql.DB
	tableName string
)

func InitLogsDatabase(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	var err error
	logsDB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open logs database: %w", err)
	}

	// SQLite serializes writes — limit to one connection to avoid SQLITE_BUSY.
	logsDB.SetMaxOpenConns(1)
	logsDB.SetMaxIdleConns(1)
	logsDB.SetConnMaxLifetime(5 * time.Minute)

	if err = logsDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping logs database: %w", err)
	}

	// Use the rollback journal (DELETE) instead of WAL.
	//
	// WAL memory-maps a -shm index file. modernc.org/sqlite (a C-to-Go
	// transpile) is built without SEH on Windows, so an I/O page fault on that
	// mapping (common when the database lives on an SMB/network share)
	// terminates the process with EXCEPTION_IN_PAGE_ERROR instead of being
	// caught and retried. With SetMaxOpenConns(1) above, WAL provides no
	// concurrency benefit either, so the rollback journal is strictly better.
	if _, err = logsDB.Exec("PRAGMA journal_mode=DELETE"); err != nil {
		return fmt.Errorf("failed to set journal mode: %w", err)
	}

	// Wait up to 5 seconds for a busy database instead of immediately failing with SQLITE_BUSY.
	if _, err = logsDB.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	tableName = fmt.Sprintf("logs_%s", timestamp)

	createTableSQL := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		level INTEGER NOT NULL,
		content TEXT NOT NULL,
		timestamp TEXT NOT NULL DEFAULT (datetime('now'))
	);`, tableName)

	_, err = logsDB.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create logs table %s: %w", tableName, err)
	}

	return nil
}

func insertLogToDB(db *sql.DB, level int, content string, timestamp string) {
	if db != nil {
		query := fmt.Sprintf(`INSERT INTO %s (level, content, timestamp) VALUES (?, ?, ?)`, tableName)
		_, err := db.Exec(query, level, content, timestamp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write log to database: %v\n", err)
		}
	}
}

func SetLogsDatabase(db *sql.DB) {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	logsDB = db
}
