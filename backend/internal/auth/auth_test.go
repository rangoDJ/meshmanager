package auth

import (
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct horse") {
		t.Error("expected password to verify")
	}
	if VerifyPassword(hash, "wrong horse") {
		t.Error("wrong password verified")
	}
	if VerifyPassword("", "correct horse") {
		t.Error("empty hash verified")
	}
	if VerifyPassword("garbage", "correct horse") {
		t.Error("malformed hash verified")
	}
}

func TestValidation(t *testing.T) {
	for _, u := range []string{"admin", "a.b-c_d", "A1"} {
		if msg := ValidateUsername(u); msg != "" {
			t.Errorf("username %q rejected: %s", u, msg)
		}
	}
	for _, u := range []string{"", "has space", "slash/", "ünicode"} {
		if ValidateUsername(u) == "" {
			t.Errorf("username %q accepted", u)
		}
	}
	if ValidatePassword("short") == "" {
		t.Error("short password accepted")
	}
	if ValidatePassword("long enough") != "" {
		t.Error("valid password rejected")
	}
}

func TestTokenHash(t *testing.T) {
	a, b := NewToken(), NewToken()
	if a == b || len(a) < 40 {
		t.Fatalf("tokens not random enough: %q %q", a, b)
	}
	if HashToken(a) != HashToken(a) || HashToken(a) == HashToken(b) {
		t.Error("token hash not deterministic or collides")
	}
}

func TestRateLimiter(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewRateLimiter(3, time.Minute)
	l.now = func() time.Time { return now }

	for range 3 {
		if limited, _ := l.Limited("ip"); limited {
			t.Fatal("limited too early")
		}
		l.Hit("ip")
	}
	limited, wait := l.Limited("ip")
	if !limited || wait != time.Minute {
		t.Fatalf("expected limit with 1m wait, got %v %v", limited, wait)
	}
	if limited, _ := l.Limited("other"); limited {
		t.Error("keys are not independent")
	}

	now = now.Add(61 * time.Second)
	if limited, _ := l.Limited("ip"); limited {
		t.Error("window did not slide")
	}

	l.Hit("ip")
	l.Reset("ip")
	if len(l.hits) != 0 {
		t.Error("reset left entries behind")
	}
}
