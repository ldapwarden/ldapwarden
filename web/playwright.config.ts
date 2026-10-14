import { defineConfig, devices } from '@playwright/test'
import { ACCOUNTS, SERVERS } from './e2e/matrix'

// End-to-end checks of what each role sees, against the real binary serving the
// real build (web/dist), on the bundled compose stack. `make test-e2e` builds
// both first; see e2e/rbac.spec.ts for what is checked and why.
//
// One server per configuration: a deployment's modules decide which sections
// exist as much as the role does, and they are read at startup.
export default defineConfig({
  testDir: './e2e',
  // The checks only read, but every project signs in against one directory and
  // one database; parallel workers would only race on the login rate limit.
  workers: 1,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    ...devices['Desktop Chrome'],
    // Wide enough for the full nav row: below md the sections move into a menu.
    viewport: { width: 1400, height: 900 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'sign-in', testMatch: /sign-in\.setup\.ts/ },
    ...SERVERS.flatMap((server) =>
      ACCOUNTS.map((account) => ({
        name: `${server.name}-${account.role}`,
        testMatch: /\.spec\.ts/,
        dependencies: ['sign-in'],
        use: {
          baseURL: server.baseURL,
          storageState: account.storageState(server),
        },
        metadata: { server: server.name, role: account.role },
      })),
    ),
  ],
  webServer: SERVERS.map((server) => ({
    // Built by `make test-e2e`; run from the repository root, where the binary
    // finds web/dist.
    command: './tmp/ldapwarden-e2e',
    cwd: '..',
    url: `${server.baseURL}/readyz`,
    reuseExistingServer: false,
    timeout: 60_000,
    env: {
      ...(process.env as Record<string, string>),
      LDAPWARDEN_DEV_MODE: '1',
      SERVER_PORT: String(server.port),
      LDAPWARDEN_PUBLIC_URL: server.baseURL,
      LDAPWARDEN_MODULES: server.modules.join(','),
      LDAP_DEFAULT_POLICY_DN: 'cn=default,ou=policies,dc=example,dc=org',
      LDAP_ACCESSLOG_BASE: 'cn=accesslog',
    },
  })),
})
