package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
)

const (
	MinPasswordLength = 8
	MaxPasswordLength = 256
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

var params = &argon2id.Params{
	Memory:      64 * 1024,
	Iterations:  2,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// Compared against when a username doesn't exist, so response timing doesn't reveal valid usernames.
var dummyHash = sync.OnceValue(func() string {
	h, _ := argon2id.CreateHash("not-a-real-password", params)
	return h
})

func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, params)
}

// VerifyPassword reports whether password matches hash. An empty hash always fails,
// but still costs the same as a real comparison.
func VerifyPassword(hash, password string) bool {
	if hash == "" {
		_, _ = argon2id.ComparePasswordAndHash(password, dummyHash())
		return false
	}
	ok, err := argon2id.ComparePasswordAndHash(password, hash)
	return err == nil && ok
}

// ValidateUsername returns a user-facing problem, or "" if the username is acceptable.
func ValidateUsername(username string) string {
	if !usernameRe.MatchString(username) {
		return "username must be 1-64 characters: letters, digits, '.', '_' or '-'"
	}
	return ""
}

func ValidatePassword(password string) string {
	n := utf8.RuneCountInString(password)
	if n < MinPasswordLength {
		return "password must be at least 8 characters"
	}
	if n > MaxPasswordLength {
		return "password must be at most 256 characters"
	}
	return ""
}

// NewToken returns a random session token for the cookie. Only its hash is stored.
func NewToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RateLimiter is an in-memory sliding window keyed by client. Fine for one API process.
type RateLimiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[string][]time.Time
}

func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{max: max, window: window, now: time.Now, hits: map[string][]time.Time{}}
}

func (l *RateLimiter) prune(key string) []time.Time {
	cutoff := l.now().Add(-l.window)
	hits := l.hits[key]
	i := 0
	for i < len(hits) && hits[i].Before(cutoff) {
		i++
	}
	hits = hits[i:]
	if len(hits) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = hits
	return hits
}

// Limited reports whether key has used up its attempts, and how long until the oldest expires.
func (l *RateLimiter) Limited(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	hits := l.prune(key)
	if len(hits) < l.max {
		return false, 0
	}
	return true, hits[0].Add(l.window).Sub(l.now())
}

func (l *RateLimiter) Hit(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.prune(key), l.now())
}

func (l *RateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
