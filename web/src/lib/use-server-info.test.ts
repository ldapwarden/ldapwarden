import { describe, expect, it } from 'vitest'
import type { ServerInfo } from './api'
import { moduleAvailable } from './use-server-info'

const info = (modules: string[], netgroupsSupported: boolean) =>
  ({ modules, capabilities: { netgroupsSupported } }) as unknown as ServerInfo

describe('moduleAvailable', () => {
  it('follows the enabled modules', () => {
    expect(moduleAvailable('sudo', info(['users', 'groups'], true))).toBe(false)
    expect(moduleAvailable('groups', info(['users', 'groups'], true))).toBe(true)
  })

  // Enabling the module is not enough: without nis.schema the API answers 501.
  it('needs the directory to hold netgroups as well', () => {
    expect(moduleAvailable('netgroups', info(['netgroups'], false))).toBe(false)
    expect(moduleAvailable('netgroups', info(['netgroups'], true))).toBe(true)
  })

  it('falls back to the default modules until server-info answers', () => {
    expect(moduleAvailable('policies', undefined)).toBe(true)
    expect(moduleAvailable('netgroups', undefined)).toBe(false)
  })
})
