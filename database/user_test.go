package database

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

// initTempDB opens a fresh user database in a temp directory and registers a
// cleanup that closes it when the test finishes.
func initTempDB(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "user.db")
	if err := InitUserDB(path); err != nil {
		t.Fatalf("InitUserDB: %v", err)
	}
	t.Cleanup(CloseUserDB)
}

func TestRegisterFirstUserClosedAfterFirst(t *testing.T) {
	initTempDB(t)

	if _, err := RegisterFirstUser("alice", "password1", "admin"); err != nil {
		t.Fatalf("first registration should succeed: %v", err)
	}

	if _, err := RegisterFirstUser("bob", "password2", "admin"); !errors.Is(err, ErrUsersExist) {
		t.Fatalf("second registration error = %v, want ErrUsersExist", err)
	}

	count, err := GetUserCount()
	if err != nil {
		t.Fatalf("GetUserCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("user count = %d, want 1", count)
	}
}

func TestRegisterFirstUserConcurrentOnlyOneWins(t *testing.T) {
	initTempDB(t)

	const n = 8
	var wg sync.WaitGroup
	results := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := RegisterFirstUser("user"+string(rune('a'+i)), "password", "admin")
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrUsersExist):
			// Expected for every registration that lost the race.
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent registrations: %d succeeded, want exactly 1", successes)
	}

	count, err := GetUserCount()
	if err != nil {
		t.Fatalf("GetUserCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("user count = %d, want 1", count)
	}
}
