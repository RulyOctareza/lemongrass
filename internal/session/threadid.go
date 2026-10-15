package session

import (
	"crypto/rand"
	"database/sql"
	"errors"
)

const (
	shortIDLength   = 6
	shortIDAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	shortIDAttempts = 20
)

type queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

// A random 6 character id that no thread in the store uses yet.
func newThreadID(q queryer) (string, error) {
	return newShortID(q, `SELECT 1 FROM lg_threads WHERE id = ?`, "thread")
}

// A random 6 character id that the exists query, given the id as its only argument, finds no row for.
func newShortID(q queryer, exists, kind string) (string, error) {
	for i := 0; i < shortIDAttempts; i++ {
		buf := make([]byte, shortIDLength)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for j, b := range buf {
			buf[j] = shortIDAlphabet[int(b)%len(shortIDAlphabet)]
		}
		id := string(buf)
		var one int
		err := q.QueryRow(exists, id).Scan(&one)
		if err == sql.ErrNoRows {
			return id, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", errors.New("session: no free " + kind + " id found")
}
