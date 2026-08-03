package postLog

import (
	"database/sql"
	"fmt"
	"os"
	"time"
)

var (
	logsDB    *sql.DB
	tableName string
)

func InitLogsDatabase(dbPath string) error {
	var err error
	logsDB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open logs database: %w", err)
	}

	if err = logsDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping logs database: %w", err)
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
		_, err := db.Exec(fmt.Sprintf(`INSERT INTO %s (level, content, timestamp) VALUES (%d, '%s', '%s')`, tableName,
			level, content, timestamp))
		// fmt.Fprintf(os.Stderr, `INSERT INTO %s (level, content, timestamp) VALUES (%d, '%s', '%s')`, tableName, level, content, timestamp)
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
