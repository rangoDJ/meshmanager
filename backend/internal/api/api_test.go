package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rangoDJ/meshmanager/internal/api"
	"github.com/rangoDJ/meshmanager/internal/config"
	"github.com/rangoDJ/meshmanager/internal/db"
	"github.com/rangoDJ/meshmanager/internal/jobs"
	"github.com/rangoDJ/meshmanager/internal/settings"
	"github.com/rangoDJ/meshmanager/internal/store"
)

var testPool *pgxpool.Pool

// TestMain uses TEST_DATABASE_URL when set (CI), otherwise starts an embedded Postgres.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		pg, u, err := startEmbedded()
		if err != nil {
			fmt.Fprintln(os.Stderr, "embedded postgres:", err)
			return 1
		}
		defer func() { _ = pg.Stop() }()
		url = u
	}

	pool, err := db.Connect(ctx, url, 30*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	testPool = pool
	return m.Run()
}

func startEmbedded() (*embeddedpostgres.EmbeddedPostgres, string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	port := uint32(l.Addr().(*net.TCPAddr).Port)
	_ = l.Close()

	runtime, err := os.MkdirTemp("", "mm-pg-*")
	if err != nil {
		return nil, "", err
	}
	cache, _ := os.UserCacheDir()
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(port).
		Username("mesh").Password("mesh").Database("mesh").
		RuntimePath(runtime).
		CachePath(filepath.Join(cache, "meshmanager-embedded-pg")).
		Logger(io.Discard))
	if err := pg.Start(); err != nil {
		return nil, "", err
	}
	return pg, fmt.Sprintf("postgres://mesh:mesh@127.0.0.1:%d/mesh?sslmode=disable", port), nil
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

// do sends a JSON request with the CSRF header and decodes a JSON response if present.
func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	return c.doHeaders(method, path, body, map[string]string{"X-Requested-With": "test"})
}

func (c *client) doHeaders(method, path string, body any, headers map[string]string) (int, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		if raw[0] == '[' {
			var list []any
			_ = json.Unmarshal(raw, &list)
			out = map[string]any{"items": list}
		} else {
			_ = json.Unmarshal(raw, &out)
		}
	}
	return resp.StatusCode, out
}

func testConfig(t *testing.T) config.Config {
	return config.Config{
		SessionTTL:        time.Hour,
		CookieSecure:      "auto",
		LibraryRoot:       t.TempDir(),
		DataDir:           t.TempDir(),
		WorkerConcurrency: 2,
	}
}

func newServer(t *testing.T) (string, *store.Store) {
	t.Helper()
	return newServerWithConfig(t, testConfig(t))
}

func newServerWithConfig(t *testing.T, cfg config.Config) (string, *store.Store) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		"TRUNCATE users, auth_sessions, jobs, workers, sources, river_job RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(testPool)
	queue, err := jobs.NewInsertClient(testPool)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.New(api.Deps{
		Config: cfg, Store: st, Queue: queue, Settings: settings.NewStore(cfg.DataDir),
	}).Handler())
	t.Cleanup(srv.Close)
	return srv.URL, st
}

// startWorker runs a River worker for the duration of the test.
func startWorker(t *testing.T, st *store.Store, cfg config.Config) {
	t.Helper()
	worker, err := jobs.NewWorkerClient(testPool, st, cfg, settings.NewStore(cfg.DataDir),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = worker.Stop(context.Background())
	})
}

func setupAdmin(t *testing.T, c *client) {
	t.Helper()
	status, body := c.do("POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "hunter2hunter2"})
	if status != http.StatusCreated {
		t.Fatalf("setup: %d %v", status, body)
	}
}

func TestHealth(t *testing.T) {
	base, _ := newServer(t)
	status, body := newClient(t, base).do("GET", "/api/health", nil)
	if status != 200 || body["database"] != "ok" {
		t.Fatalf("health: %d %v", status, body)
	}
}

func TestSetupFlow(t *testing.T) {
	base, _ := newServer(t)
	c := newClient(t, base)

	_, body := c.do("GET", "/api/auth/status", nil)
	if body["setup_required"] != true || body["user"] != nil {
		t.Fatalf("fresh status: %v", body)
	}

	if status, _ := c.do("POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "short"}); status != 400 {
		t.Errorf("short password: %d", status)
	}
	if status, _ := c.do("POST", "/api/auth/setup", map[string]string{"username": "bad name", "password": "longenough"}); status != 400 {
		t.Errorf("bad username: %d", status)
	}

	setupAdmin(t, c)

	_, body = c.do("GET", "/api/auth/status", nil)
	user, _ := body["user"].(map[string]any)
	if body["setup_required"] != false || user["username"] != "admin" {
		t.Fatalf("status after setup: %v", body)
	}
	if _, ok := user["password_hash"]; ok {
		t.Error("password hash leaked in API response")
	}

	if status, _ := newClient(t, base).do("POST", "/api/auth/setup",
		map[string]string{"username": "other", "password": "longenough"}); status != 409 {
		t.Errorf("second setup: %d, want 409", status)
	}
}

func TestConcurrentSetupCreatesOneAdmin(t *testing.T) {
	base, st := newServer(t)
	var wg sync.WaitGroup
	codes := make(chan int, 5)
	for i := range 5 {
		wg.Go(func() {
			status, _ := newClient(t, base).do("POST", "/api/auth/setup",
				map[string]string{"username": fmt.Sprintf("admin%d", i), "password": "longenough"})
			codes <- status
		})
	}
	wg.Wait()
	close(codes)
	created := 0
	for code := range codes {
		if code == 201 {
			created++
		}
	}
	n, _ := st.CountUsers(context.Background())
	if created != 1 || n != 1 {
		t.Fatalf("created=%d users=%d, want exactly 1", created, n)
	}
}

func TestLoginLogout(t *testing.T) {
	base, _ := newServer(t)
	setupAdmin(t, newClient(t, base))
	c := newClient(t, base)

	if status, _ := c.do("GET", "/api/auth/me", nil); status != 401 {
		t.Fatalf("me before login: %d", status)
	}
	if status, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "nope"}); status != 401 {
		t.Fatalf("bad password: %d", status)
	}
	if status, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "ghost", "password": "nope"}); status != 401 {
		t.Fatalf("unknown user: %d", status)
	}
	if status, body := c.do("POST", "/api/auth/login", map[string]string{"username": "ADMIN", "password": "hunter2hunter2"}); status != 200 {
		t.Fatalf("login (case-insensitive username): %d %v", status, body)
	}
	if status, body := c.do("GET", "/api/auth/me", nil); status != 200 || body["username"] != "admin" {
		t.Fatalf("me: %d %v", status, body)
	}
	if status, _ := c.do("POST", "/api/auth/logout", nil); status != 204 {
		t.Fatalf("logout: %d", status)
	}
	if status, _ := c.do("GET", "/api/auth/me", nil); status != 401 {
		t.Fatalf("me after logout: %d", status)
	}
}

func TestCSRFHeaderRequired(t *testing.T) {
	base, _ := newServer(t)
	c := newClient(t, base)
	status, _ := c.doHeaders("POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "longenough"}, nil)
	if status != 403 {
		t.Fatalf("POST without X-Requested-With: %d, want 403", status)
	}
}

func TestLoginRateLimit(t *testing.T) {
	base, _ := newServer(t)
	setupAdmin(t, newClient(t, base))
	c := newClient(t, base)
	for i := range 10 {
		if status, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "wrong"}); status != 401 {
			t.Fatalf("attempt %d: %d", i, status)
		}
	}
	if status, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "hunter2hunter2"}); status != 429 {
		t.Fatalf("11th attempt: %d, want 429", status)
	}
}

func TestChangePasswordSignsOutOtherSessions(t *testing.T) {
	base, _ := newServer(t)
	first := newClient(t, base)
	setupAdmin(t, first)
	second := newClient(t, base)
	if status, _ := second.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "hunter2hunter2"}); status != 200 {
		t.Fatal("second login failed")
	}

	if status, _ := first.do("POST", "/api/auth/password",
		map[string]string{"current_password": "wrong", "new_password": "newpassword1"}); status != 400 {
		t.Fatalf("wrong current password: %d", status)
	}
	if status, _ := first.do("POST", "/api/auth/password",
		map[string]string{"current_password": "hunter2hunter2", "new_password": "newpassword1"}); status != 204 {
		t.Fatalf("change password: %d", status)
	}

	if status, _ := first.do("GET", "/api/auth/me", nil); status != 200 {
		t.Errorf("current session was signed out: %d", status)
	}
	if status, _ := second.do("GET", "/api/auth/me", nil); status != 401 {
		t.Errorf("other session still valid: %d", status)
	}
	if status, _ := newClient(t, base).do("POST", "/api/auth/login",
		map[string]string{"username": "admin", "password": "newpassword1"}); status != 200 {
		t.Errorf("login with new password: %d", status)
	}
}

func TestPingJobRunsOnWorker(t *testing.T) {
	base, st := newServer(t)
	c := newClient(t, base)
	setupAdmin(t, c)

	startWorker(t, st, testConfig(t))

	status, job := c.do("POST", "/api/jobs/ping", nil)
	if status != 202 || job["state"] != "queued" {
		t.Fatalf("enqueue: %d %v", status, job)
	}
	path := fmt.Sprintf("/api/jobs/%v", job["id"])

	deadline := time.Now().Add(20 * time.Second)
	for {
		_, job = c.do("GET", path, nil)
		if job["state"] == "done" {
			break
		}
		if job["state"] == "failed" || time.Now().After(deadline) {
			t.Fatalf("job did not finish: %v", job)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if msg, _ := job["message"].(string); !strings.HasPrefix(msg, "pong from ") || job["progress"] != 1.0 {
		t.Errorf("unexpected finished job: %v", job)
	}

	_, list := c.do("GET", "/api/jobs", nil)
	if items, _ := list["items"].([]any); len(items) != 1 {
		t.Errorf("job list: %v", list)
	}
}

func TestUnknownAPIRouteIsJSON404(t *testing.T) {
	base, _ := newServer(t)
	status, body := newClient(t, base).do("GET", "/api/nope", nil)
	if status != 404 || body["error"] == nil {
		t.Fatalf("got %d %v", status, body)
	}
}
