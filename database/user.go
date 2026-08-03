package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// UserDB is the global database handle for user data.
var UserDB *sql.DB

// InitUserDB opens (or creates) the user database and initializes the users table.
func InitUserDB(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	var err error
	UserDB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open user database: %w", err)
	}

	UserDB.SetMaxOpenConns(1) // SQLite serializes writes.
	UserDB.SetMaxIdleConns(1)
	UserDB.SetConnMaxLifetime(5 * time.Minute)

	if err := UserDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping user database: %w", err)
	}

	if err := initUserTable(); err != nil {
		return fmt.Errorf("failed to initialize user table: %w", err)
	}

	return nil
}

func initUserTable() error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL,
		level TEXT NOT NULL DEFAULT 'admin',
		register_date TEXT NOT NULL
	);`
	_, err := UserDB.Exec(createTableSQL)
	return err
}

// HashPassword returns the SHA-256 hex hash of a password.
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// CreateUser inserts a new user into the database.
func CreateUser(username, password, level string) (int64, error) {
	hashedPassword := HashPassword(password)
	registerDate := time.Now().Format("2006-01-02 15:04:05")

	result, err := UserDB.Exec(
		"INSERT INTO users (username, password, level, register_date) VALUES (?, ?, ?, ?)",
		username, hashedPassword, level, registerDate,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create user: %w", err)
	}

	return result.LastInsertId()
}

// GetUserByUsername retrieves a user by username and validates the password.
func GetUserByUsername(username, password string) (int64, string, string, string, error) {
	hashedPassword := HashPassword(password)

	var id int64
	var dbUsername string
	var level string
	var registerDate string

	err := UserDB.QueryRow(
		"SELECT id, username, level, register_date FROM users WHERE username = ? AND password = ?",
		username, hashedPassword,
	).Scan(&id, &dbUsername, &level, &registerDate)

	if err == sql.ErrNoRows {
		return 0, "", "", "", fmt.Errorf("invalid username or password")
	}
	if err != nil {
		return 0, "", "", "", fmt.Errorf("database error: %w", err)
	}

	return id, dbUsername, level, registerDate, nil
}

// GetUserCount returns the total number of users in the database.
func GetUserCount() (int, error) {
	var count int
	err := UserDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

// GetUserByID retrieves a user by their ID.
func GetUserByID(id int64) (string, string, string, error) {
	var username string
	var level string
	var registerDate string

	err := UserDB.QueryRow(
		"SELECT username, level, register_date FROM users WHERE id = ?",
		id,
	).Scan(&username, &level, &registerDate)

	if err == sql.ErrNoRows {
		return "", "", "", fmt.Errorf("user not found")
	}
	if err != nil {
		return "", "", "", fmt.Errorf("database error: %w", err)
	}

	return username, level, registerDate, nil
}

// CloseUserDB closes the user database connection.
func CloseUserDB() {
	if UserDB != nil {
		UserDB.Close()
	}
}
