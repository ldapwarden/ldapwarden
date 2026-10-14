import { test as setup, expect } from '@playwright/test'
import { ACCOUNTS, SERVERS } from './matrix'

// One session per account and server, signed in through the API and stored as
// the browser state the matrix projects start from. The session cookie is
// HttpOnly, so the API's Set-Cookie is the only way to obtain it.
for (const server of SERVERS) {
  for (const account of ACCOUNTS) {
    setup(`sign in as ${account.role} on ${server.name}`, async ({ playwright }) => {
      const request = await playwright.request.newContext({ baseURL: server.baseURL })
      const response = await request.post('/api/auth/login', {
        data: { username: account.username, password: account.password },
      })
      expect(response.status(), await response.text()).toBe(200)
      const session = await (await request.get('/api/auth/me')).json()
      expect(session.roleName).toBe(account.role)
      await request.storageState({ path: account.storageState(server) })
      await request.dispose()
    })
  }
}
