// Command meshmanager runs the API server, the background worker, or admin tasks.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // the distroless image has no zoneinfo; TZ still works

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"golang.org/x/term"

	"github.com/rangoDJ/meshmanager/internal/api"
	"github.com/rangoDJ/meshmanager/internal/auth"
	"github.com/rangoDJ/meshmanager/internal/config"
	"github.com/rangoDJ/meshmanager/internal/db"
	"github.com/rangoDJ/meshmanager/internal/jobs"
	"github.com/rangoDJ/meshmanager/internal/settings"
	"github.com/rangoDJ/meshmanager/internal/store"
	"github.com/rangoDJ/meshmanager/internal/version"
	"github.com/rangoDJ/meshmanager/internal/web"
)

const usage = `Usage: meshmanager <command> [flags]

Commands:
  serve           run migrations, then the HTTP API and web UI
  worker          run background jobs only (optional: serve already runs them)
  migrate         apply database migrations and exit
  reset-password  set a new password for a user (-username NAME)
  healthcheck     exit 0 if the local server reports healthy
  version         print the version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	if cmd == "version" {
		fmt.Println(version.Version)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))

	if cmd != "healthcheck" {
		if err := dropPrivileges(cfg.DataDir); err != nil {
			slog.Error("cannot switch to PUID/PGID", "err", err)
			os.Exit(1)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "serve":
		err = serve(ctx, cfg)
	case "worker":
		err = worker(ctx, cfg)
	case "migrate":
		err = migrate(ctx, cfg)
	case "reset-password":
		err = resetPassword(ctx, cfg, args)
	case "healthcheck":
		err = healthcheck(cfg)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		slog.Error(cmd+" failed", "err", err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, cfg config.Config) error {
	pool, err := db.Connect(ctx, cfg.DatabaseURL, time.Minute)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	st := store.New(pool)
	if err := bootstrapAdmin(ctx, cfg, st); err != nil {
		return err
	}
	set := settings.NewStore(cfg.DataDir)
	if err := set.EnsureFile(); err != nil {
		slog.Warn("cannot write the settings file; make DATA_DIR writable to save settings",
			"path", set.Path(), "err", err)
	}

	// By default this process also runs background jobs, so a deployment is just this
	// container and Postgres. Separate `worker` containers can add capacity.
	var queue *river.Client[pgx.Tx]
	if cfg.RunWorker {
		queue, err = jobs.NewWorkerClient(pool, st, cfg, set, slog.Default())
	} else {
		queue, err = jobs.NewInsertClient(pool)
	}
	if err != nil {
		return err
	}
	jobsCtx, stopJobs := context.WithCancel(context.Background())
	defer stopJobs()
	jobsDone := make(chan error, 1)
	if cfg.RunWorker {
		go func() { jobsDone <- runJobs(jobsCtx, queue, st, cfg) }()
	} else {
		jobsDone <- nil
	}

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: api.New(api.Deps{
			Config:   cfg,
			Store:    st,
			Queue:    queue,
			Settings: set,
			Web:      web.Handler(),
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr, "version", version.Version)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case err := <-jobsDone:
		return fmt.Errorf("background jobs stopped: %w", err)
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	stopJobs()
	if jobsErr := <-jobsDone; err == nil {
		err = jobsErr
	}
	return err
}

// bootstrapAdmin creates the admin from ADMIN_USERNAME / ADMIN_PASSWORD on first start.
func bootstrapAdmin(ctx context.Context, cfg config.Config, st *store.Store) error {
	if cfg.AdminUsername == "" {
		return nil
	}
	if msg := auth.ValidateUsername(cfg.AdminUsername); msg != "" {
		return fmt.Errorf("ADMIN_USERNAME: %s", msg)
	}
	if msg := auth.ValidatePassword(cfg.AdminPassword); msg != "" {
		return fmt.Errorf("ADMIN_PASSWORD: %s", msg)
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	_, err = st.CreateFirstAdmin(ctx, cfg.AdminUsername, hash)
	if errors.Is(err, store.ErrSetupDone) {
		return nil
	}
	if err == nil {
		slog.Info("admin account created from environment", "username", cfg.AdminUsername)
	}
	return err
}

func worker(ctx context.Context, cfg config.Config) error {
	pool, err := db.Connect(ctx, cfg.DatabaseURL, time.Minute)
	if err != nil {
		return err
	}
	defer pool.Close()
	st := store.New(pool)
	client, err := jobs.NewWorkerClient(pool, st, cfg, settings.NewStore(cfg.DataDir), slog.Default())
	if err != nil {
		return err
	}
	return runJobs(ctx, client, st, cfg)
}

// runJobs processes background jobs until ctx is cancelled, sending a heartbeat so the
// dashboard can list running workers.
func runJobs(ctx context.Context, client *river.Client[pgx.Tx], st *store.Store, cfg config.Config) error {
	if err := client.Start(ctx); err != nil {
		return err
	}

	hostname, _ := os.Hostname()
	idBytes := make([]byte, 4)
	_, _ = rand.Read(idBytes)
	info := store.WorkerInfo{
		ID:        hostname + "-" + hex.EncodeToString(idBytes),
		Hostname:  hostname,
		Version:   version.Version,
		StartedAt: time.Now(),
	}
	slog.Info("background jobs started", "id", info.ID, "concurrency", cfg.WorkerConcurrency)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if err := st.WorkerHeartbeat(ctx, info); err != nil && ctx.Err() == nil {
			slog.Warn("worker heartbeat", "err", err)
		}
		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
		}
		break
	}

	slog.Info("background jobs stopping")
	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Stop(stopCtx); err != nil {
		slog.Warn("graceful stop timed out, cancelling running jobs", "err", err)
		_ = client.StopAndCancel(context.Background())
	}
	return st.RemoveWorker(context.Background(), info.ID)
}

func migrate(ctx context.Context, cfg config.Config) error {
	pool, err := db.Connect(ctx, cfg.DatabaseURL, time.Minute)
	if err != nil {
		return err
	}
	defer pool.Close()
	return db.Migrate(ctx, pool)
}

// resetPassword reads the new password from MM_NEW_PASSWORD or prompts for it,
// then signs the user out everywhere.
func resetPassword(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
	username := fs.String("username", "", "user whose password to reset")
	_ = fs.Parse(args)
	if *username == "" {
		return errors.New("-username is required")
	}

	password := os.Getenv("MM_NEW_PASSWORD")
	if password == "" {
		var err error
		if password, err = promptPassword(); err != nil {
			return err
		}
	}
	if msg := auth.ValidatePassword(password); msg != "" {
		return errors.New(msg)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL, 10*time.Second)
	if err != nil {
		return err
	}
	defer pool.Close()
	st := store.New(pool)

	u, err := st.GetUserByUsername(ctx, *username)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no user named %q", *username)
	}
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := st.SetPassword(ctx, u.ID, hash); err != nil {
		return err
	}
	if err := st.DeleteUserSessions(ctx, u.ID); err != nil {
		return err
	}
	fmt.Printf("Password for %s updated; all sessions signed out.\n", u.Username)
	return nil
}

func promptPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no password given on stdin")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, "New password: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Repeat password: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("passwords do not match")
	}
	return string(first), nil
}

// healthcheck lets the distroless image check itself without curl.
func healthcheck(cfg config.Config) error {
	host, port, err := net.SplitHostPort(cfg.ListenAddr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/api/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %s", resp.Status)
	}
	return nil
}
