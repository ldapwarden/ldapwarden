//go:build integration

package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

	// Point at the host ports docker-compose publishes for the bundled stack.
	setEnvIfUnset(t, "DATABASE_URL", "postgres://ldapwarden:ldapwarden@localhost:5432/ldapwarden?sslmode=disable")
	setEnvIfUnset(t, "REDIS_URL", "redis://localhost:6379")
	setEnvIfUnset(t, "LDAP_URL", "ldap://localhost:389")
	// Tests rely on the bundled compose credentials; bypass ValidateSecrets.
	setEnvIfUnset(t, "LDAPWARDEN_DEV_MODE", "1")

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
	rbacService := rbac.NewRBAC(cfg.App.AdminGroup)
	mailer := mail.NewMailer(&cfg.Mail, cfg.App.Organization)
	auditLogger := audit.NewLogger(pool, mailer, cfg.App.AuditNotifyEmails)
	passwordResetService := passwordreset.NewService(pool)
	// scheduler is constructed but never Start()-ed: tests don't need cron jobs.
	sched := scheduler.New(cfg, ldapClient, mailer, pool, auditLogger, passwordResetService)

	server := NewServer(ldapClient, authService, auditLogger, rbacService, cfg, mailer, passwordResetService, sched, nil)
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

// doJSON sends an HTTP request to the test server. The body is JSON-encoded if
// non-nil; the response body is fully read and returned alongside the response
// object (with its body already closed).
func doJSON(t *testing.T, env *testEnv, method, path string, body any, token string) (*http.Response, []byte) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, env.Server.URL+path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, respBody
}

// loginAs performs a login and returns the parsed response. Fatals on any
// non-200; for tests asserting on failure paths, call doJSON directly.
func loginAs(t *testing.T, env *testEnv, username, password string) *auth.LoginResponse {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodPost, "/api/auth/login",
		map[string]string{"username": username, "password": password}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login as %s: status=%d body=%s", username, resp.StatusCode, body)
	}
	var lr auth.LoginResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		t.Fatalf("unmarshal login response: %v (body=%s)", err, body)
	}
	return &lr
}

// userPath builds the URL-encoded path for a user DN as accepted by the API.
func userPath(dn string) string {
	return "/api/users/" + url.QueryEscape(dn)
}

// groupPath builds the URL-encoded path for a group DN as accepted by the API.
func groupPath(dn string) string {
	return "/api/groups/" + url.QueryEscape(dn)
}

// testIDNumber returns a UID/GID number derived from the random suffix, in a
// range above the seed data (which uses 1000-1999). LDAP itself does not
// enforce posixAccount uidNumber uniqueness, so collisions across runs are
// harmless — the DN is what guarantees per-test isolation.
func testIDNumber(suffix string) int {
	n, _ := strconv.ParseUint(suffix, 16, 64)
	return 60000 + int(n%10000)
}

// createTestUser creates a user via the API, registers a cleanup that deletes
// it, and returns the parsed user. Fatals on failure.
func createTestUser(t *testing.T, env *testEnv, token string) *ldap.User {
	t.Helper()
	suffix := uniqueSuffix(t)
	uid := "testuser-" + suffix
	n := testIDNumber(suffix)

	resp, body := doJSON(t, env, http.MethodPost, "/api/users/", map[string]any{
		"uid":       uid,
		"givenName": "Test",
		"sn":        "User-" + suffix,
		"uidNumber": n,
		"gidNumber": n,
		"mail":      uid + "@test.example",
		"password":  "testpass",
	}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: status=%d body=%s", resp.StatusCode, body)
	}
	var user ldap.User
	if err := json.Unmarshal(body, &user); err != nil {
		t.Fatalf("unmarshal user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, userPath(user.DN), nil, token)
	})
	return &user
}

// createTestGroup creates a group via the API, registers a cleanup that
// deletes it, and returns the parsed group. Fatals on failure.
func createTestGroup(t *testing.T, env *testEnv, token string) *ldap.Group {
	t.Helper()
	suffix := uniqueSuffix(t)
	cn := "testgroup-" + suffix

	resp, body := doJSON(t, env, http.MethodPost, "/api/groups/", map[string]any{
		"cn":          cn,
		"gidNumber":   testIDNumber(suffix),
		"description": "test group " + suffix,
	}, token)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create group: status=%d body=%s", resp.StatusCode, body)
	}
	var group ldap.Group
	if err := json.Unmarshal(body, &group); err != nil {
		t.Fatalf("unmarshal group: %v", err)
	}
	t.Cleanup(func() {
		_, _ = doJSON(t, env, http.MethodDelete, groupPath(group.DN), nil, token)
	})
	return &group
}
