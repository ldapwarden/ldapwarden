//go:build integration

package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/auth"
	"github.com/ldapwarden/ldapwarden/internal/config"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
	"github.com/ldapwarden/ldapwarden/internal/mail"
	"github.com/ldapwarden/ldapwarden/internal/passwordreset"
	"github.com/ldapwarden/ldapwarden/internal/rbac"
	"github.com/ldapwarden/ldapwarden/internal/scheduler"
)

type testEnv struct {
	Server *httptest.Server
	LDAP   *ldap.Client
	Pool   *pgxpool.Pool
	Redis  *redis.Client
	Config *config.Config
}

// setupTestServer boots the full HTTP stack against the services declared in
// docker-compose.yaml. The test is skipped when LDAPWARDEN_TEST_INTEGRATION is
// not "1" or any of postgres / redis / OpenLDAP cannot be reached.
func setupTestServer(t *testing.T) *testEnv {
	t.Helper()

	if os.Getenv("LDAPWARDEN_TEST_INTEGRATION") != "1" {
		t.Skip("set LDAPWARDEN_TEST_INTEGRATION=1 (and start docker compose) to run integration tests")
	}

	// config defaults target a non-standard local setup; align with the ports
	// docker-compose actually publishes.
	setEnvIfUnset(t, "DATABASE_URL", "postgres://ldapwarden:ldapwarden@localhost:5432/ldapwarden?sslmode=disable")
	setEnvIfUnset(t, "REDIS_URL", "redis://localhost:6379")
	setEnvIfUnset(t, "LDAP_URL", "ldap://localhost:389")

	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		t.Fatalf("postgres pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres unreachable at %s: %v", cfg.Database.URL, err)
	}

	if err := runMigrations(t, cfg.Database.URL); err != nil {
		pool.Close()
		t.Fatalf("run migrations: %v", err)
	}

	redisOpts, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		pool.Close()
		t.Fatalf("redis URL parse: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		_ = redisClient.Close()
		pool.Close()
		t.Skipf("redis unreachable at %s: %v", cfg.Redis.URL, err)
	}

	ldapClient := ldap.NewClient(cfg.LDAP)
	// Force a bind via a base-DN lookup so we fail fast if LDAP is down or seed
	// data is missing.
	if _, err := ldapClient.GetEntry(cfg.LDAP.BaseDN, []string{"dc"}); err != nil {
		ldapClient.Close()
		_ = redisClient.Close()
		pool.Close()
		t.Skipf("LDAP unreachable at %s: %v", cfg.LDAP.URL, err)
	}

	sessionStore := auth.NewRedisSessionStore(redisClient)
	authService := auth.NewAuthService(ldapClient, sessionStore, cfg.Session.TTL, cfg.App.AdminGroup)
	auditLogger := audit.NewLogger(pool)
	rbacService := rbac.NewRBAC(cfg.App.AdminGroup)
	mailer := mail.NewMailer(&cfg.Mail, cfg.App.Organization)
	passwordResetService := passwordreset.NewService(pool)
	// scheduler is constructed but never Start()-ed: tests don't need cron jobs.
	sched := scheduler.New(cfg, ldapClient, mailer, pool, auditLogger)

	server := NewServer(ldapClient, authService, auditLogger, rbacService, cfg, mailer, passwordResetService, sched)
	httpSrv := httptest.NewServer(server)

	t.Cleanup(func() {
		httpSrv.Close()
		ldapClient.Close()
		_ = redisClient.Close()
		pool.Close()
	})

	return &testEnv{
		Server: httpSrv,
		LDAP:   ldapClient,
		Pool:   pool,
		Redis:  redisClient,
		Config: cfg,
	}
}

// runMigrations applies the SQL migrations under db/migrations using the same
// golang-migrate package the production binary uses, so reruns are idempotent
// and ERROR-free.
func runMigrations(t *testing.T, dbURL string) error {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("cannot locate test file path")
	}
	src := "file://" + filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations")
	m, err := migrate.New(src, dbURL)
	if err != nil {
		return err
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			t.Logf("migrate close: src=%v db=%v", srcErr, dbErr)
		}
	}()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// setEnvIfUnset sets key=val for the duration of the test only if the variable
// is not already defined in the environment.
func setEnvIfUnset(t *testing.T, key, val string) {
	t.Helper()
	if _, ok := os.LookupEnv(key); ok {
		return
	}
	t.Setenv(key, val)
}

// uniqueSuffix returns a short hex string for naming test entities so reruns
// and parallel tests don't collide on the shared LDAP server.
func uniqueSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}
