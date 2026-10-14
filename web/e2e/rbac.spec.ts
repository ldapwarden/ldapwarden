import { test as base, expect, type APIRequestContext, type Page, type Request } from '@playwright/test'

// What a session sees must match what the API lets it do, in both directions:
// no control that leads to a refusal, and no allowed feature hidden. Both have
// shipped — a palette linking to pages that redirect, and a read-only session
// silently losing the netgroups tab because a probe behind it got a 403.
//
// The expectations are not written down here. They come from the API, asked
// with the same session: a section should be listed when its list endpoint
// answers, a create button shown when its POST is permitted. A table of roles
// and permissions in this file would be one more copy of internal/rbac to drift
// from; this way the only rule under test is "the UI agrees with the server".

interface Section {
  key: string
  label: string
  path: string
  module?: string
  // A capability the directory must have (server-info), as netgroups need
  // nis.schema.
  capability?: 'netgroupsSupported'
  list: string
  create?: { path: string; label: string; post: string }
  importPath?: string
  importLabel?: string
  selectAll?: string
  // A seeded entry, for the detail page.
  detail?: string
}

const SECTIONS: Section[] = [
  {
    key: 'users', label: 'Users', path: '/users', module: 'users', list: '/api/users?limit=1',
    create: { path: '/users/new', label: 'New User', post: '/api/users/' },
    importPath: '/users/import', importLabel: 'Import Users',
    selectAll: 'Select all users', detail: 'uid=jdoe,ou=People,dc=example,dc=org',
  },
  {
    key: 'groups', label: 'Groups', path: '/groups', module: 'groups', list: '/api/groups?limit=1',
    create: { path: '/groups/new', label: 'New Group', post: '/api/groups/' },
    importPath: '/groups/import', importLabel: 'Import Groups',
    selectAll: 'Select all groups', detail: 'cn=developers,ou=Groups,dc=example,dc=org',
  },
  {
    key: 'sudo', label: 'Sudo Roles', path: '/sudo-roles', module: 'sudo', list: '/api/sudo-roles?limit=1',
    create: { path: '/sudo-roles/new', label: 'New Sudo Role', post: '/api/sudo-roles/' },
    selectAll: 'Select all sudo roles', detail: 'cn=dev-restart,ou=sudoers,dc=example,dc=org',
  },
  {
    key: 'netgroups', label: 'Netgroups', path: '/netgroups', module: 'netgroups', capability: 'netgroupsSupported',
    list: '/api/netgroups?limit=1',
    create: { path: '/netgroups/new', label: 'New Netgroup', post: '/api/netgroups/' },
    importPath: '/netgroups/import', importLabel: 'Import Netgroups',
    selectAll: 'Select all netgroups', detail: 'cn=ops,ou=netgroups,dc=example,dc=org',
  },
  {
    key: 'policies', label: 'Password Policies', path: '/password-policies', module: 'policies',
    list: '/api/password-policies?limit=1',
    create: { path: '/password-policies/new', label: 'New Password Policy', post: '/api/password-policies/' },
    selectAll: 'Select all password policies', detail: 'cn=default,ou=policies,dc=example,dc=org',
  },
  { key: 'audit', label: 'Audit Logs', path: '/audit-logs', list: '/api/audit-logs?limit=1' },
]

// What the server says this session may do.
interface Verdict {
  section: Section
  reachable: boolean
  canCreate: boolean
}

interface Oracle {
  sections: Verdict[]
  admin: boolean
}

// probeRead: a GET the list page itself makes. 200 means the section works for
// this session; 403 that the permission is missing; 501 that the directory
// lacks what the section needs. Anything else is a broken server, not a verdict.
async function probeRead(request: APIRequestContext, path: string): Promise<boolean> {
  const response = await request.get(path)
  expect([200, 403, 501], `GET ${path}`).toContain(response.status())
  return response.status() === 200
}

// probeWrite asks whether a POST is permitted without writing anything: the
// body is not JSON, so a permitted request stops at decoding (400) while a
// refused one stops at the permission check (403), which runs first. A 2xx
// would mean the probe wrote something, and fails the test.
async function probeWrite(request: APIRequestContext, baseURL: string, path: string): Promise<boolean> {
  const response = await request.post(path, {
    data: '{',
    headers: { 'Content-Type': 'application/json', Origin: baseURL },
  })
  expect([400, 403], `POST ${path} with a malformed body`).toContain(response.status())
  return response.status() === 400
}

async function askServer(request: APIRequestContext, baseURL: string): Promise<Oracle> {
  const info = await (await request.get('/api/server-info')).json()
  const modules: string[] = info.modules ?? []
  const sections: Verdict[] = []
  for (const section of SECTIONS) {
    const enabled =
      (!section.module || modules.includes(section.module)) &&
      (!section.capability || info.capabilities?.[section.capability] === true)
    const reachable = enabled && (await probeRead(request, section.list))
    const canCreate = reachable && !!section.create && (await probeWrite(request, baseURL, section.create.post))
    sections.push({ section, reachable, canCreate })
  }
  const admin = (await request.get('/api/admin/config')).status() === 200
  return { sections, admin }
}

// Every API call the pages make is watched: a role that may open a page must
// not have that page fail half its requests behind its back. That is how the
// lost netgroups tab looked from the outside — a 403 nobody saw.
const test = base.extend<{ oracle: Oracle; failedCalls: string[] }>({
  oracle: async ({ request, baseURL }, use) => {
    await use(await askServer(request, baseURL!))
  },
  failedCalls: [
    async ({ page }, use) => {
      const failed: string[] = []
      trackAPICalls(page)
      page.on('response', (response) => {
        const url = new URL(response.url())
        if (url.pathname.startsWith('/api/') && response.status() >= 400) {
          failed.push(`${response.request().method()} ${url.pathname}${url.search} → ${response.status()}`)
        }
      })
      await use(failed)
      expect(failed, 'API calls that failed while the pages rendered').toEqual([])
    },
    { auto: true },
  ],
})

// Mirrors encodeDN in src/lib/utils.ts: the detail routes take the DN as
// unpadded base64url.
function encodeDN(dn: string): string {
  return Buffer.from(dn).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

// settle waits until the page has made its API calls: none in flight for a
// moment. Not "networkidle", which some pages never reach — they poll — and
// which then stalls each visit for its whole timeout.
const inFlight = new WeakMap<Page, Set<Request>>()

function trackAPICalls(page: Page) {
  const pending = new Set<Request>()
  inFlight.set(page, pending)
  const isAPI = (request: Request) => new URL(request.url()).pathname.startsWith('/api/')
  page.on('request', (request) => {
    if (isAPI(request)) pending.add(request)
  })
  page.on('requestfinished', (request) => pending.delete(request))
  page.on('requestfailed', (request) => pending.delete(request))
}

async function settle(page: Page) {
  await page.waitForLoadState('load')
  let quietSince = Date.now()
  const deadline = Date.now() + 15_000
  while (Date.now() - quietSince < 300) {
    const pending = inFlight.get(page) ?? new Set()
    if (pending.size > 0) quietSince = Date.now()
    if (Date.now() > deadline) {
      const urls = [...pending].map((r) => `${r.method()} ${new URL(r.url()).pathname}${new URL(r.url()).search}`)
      throw new Error(`API calls still in flight after 15s: ${urls.join(', ')}`)
    }
    await page.waitForTimeout(50)
  }
}

test('the nav lists exactly the sections the session can use', async ({ page, oracle }) => {
  await page.goto('/')
  await settle(page)

  const hrefs = await page.locator('nav a[href]').evaluateAll((links) =>
    links.map((link) => new URL((link as HTMLAnchorElement).href).pathname),
  )
  const listed = SECTIONS.filter((s) => hrefs.includes(s.path)).map((s) => s.key)
  const expected = oracle.sections.filter((v) => v.reachable).map((v) => v.section.key)
  expect(listed.sort()).toEqual(expected.sort())

  await page.getByRole('button', { name: 'Account menu' }).click()
  await expect(page.getByRole('menuitem', { name: 'Administration' })).toHaveCount(oracle.admin ? 1 : 0)
})

test('the command palette offers what the nav does, and only creations the API permits', async ({ page, oracle }) => {
  await page.goto('/')
  await settle(page)
  await page.getByRole('button', { name: /Search/ }).first().click()
  const palette = page.getByRole('listbox')

  const goTo = await palette.getByRole('group', { name: 'Go to' }).getByRole('option').allInnerTexts()
  expect(goTo.map((t) => t.trim()).sort()).toEqual(
    ['Dashboard', ...oracle.sections.filter((v) => v.reachable).map((v) => v.section.label), ...(oracle.admin ? ['Administration'] : [])].sort(),
  )

  const createGroup = palette.getByRole('group', { name: 'Create' })
  const creations = (await createGroup.count()) ? await createGroup.getByRole('option').allInnerTexts() : []
  const expectedCreations = oracle.sections.flatMap((v) =>
    v.canCreate ? [v.section.create!.label, ...(v.section.importLabel ? [v.section.importLabel] : [])] : [],
  )
  expect(creations.map((t) => t.trim()).sort()).toEqual(expectedCreations.sort())
})

test('list pages offer create, import and selection only when the API permits writes', async ({ page, oracle }) => {
  for (const { section, reachable, canCreate } of oracle.sections) {
    if (!reachable || !section.create) continue
    await test.step(section.path, async () => {
      await page.goto(section.path)
      await settle(page)
      const main = page.locator('main')
      await expect(main.locator(`a[href="${section.create!.path}"]`).first()).toBeVisible({ visible: canCreate })
      if (section.importPath) {
        await expect(main.locator(`a[href="${section.importPath}"]`).first()).toBeVisible({ visible: canCreate })
      }
      if (section.selectAll) {
        await expect(page.getByRole('checkbox', { name: section.selectAll })).toHaveCount(canCreate ? 1 : 0)
      }
    })
  }
})

test('detail pages are read-only exactly when the API refuses writes', async ({ page, oracle }) => {
  for (const { section, reachable, canCreate } of oracle.sections) {
    if (!reachable || !section.detail) continue
    await test.step(section.detail, async () => {
      await page.goto(`${section.path}/${encodeDN(section.detail!)}`)
      await settle(page)
      await expect(page.getByText(/You have read-only access/).first()).toBeVisible({ visible: !canCreate })
    })
  }
})

test('pages the session cannot use redirect when opened by URL', async ({ page, oracle }) => {
  const refused: string[] = []
  for (const { section, reachable, canCreate } of oracle.sections) {
    if (!reachable) refused.push(section.path)
    if (!canCreate && section.create) refused.push(section.create.path)
    if (!canCreate && section.importPath) refused.push(section.importPath)
  }
  if (!oracle.admin) refused.push('/admin')

  for (const path of refused) {
    await test.step(path, async () => {
      await page.goto(path)
      await settle(page)
      expect.soft(new URL(page.url()).pathname, `${path} rendered instead of redirecting`).not.toBe(path)
    })
  }
})
