import { describe, expect, it } from 'vitest'
import { pageTitle } from './page-title'
import { encodeDN } from './utils'

describe('pageTitle', () => {
  it('names the section', () => {
    expect(pageTitle('/users')).toBe('Users · LDAP Warden')
    expect(pageTitle('/netgroups')).toBe('Netgroups · LDAP Warden')
    expect(pageTitle('/admin')).toBe('Administration · LDAP Warden')
  })

  it('falls back to the app name on the dashboard and unknown paths', () => {
    expect(pageTitle('/')).toBe('LDAP Warden')
    expect(pageTitle('/nowhere')).toBe('LDAP Warden')
  })

  it('names the entry on a detail page', () => {
    const dn = 'uid=jdoe,ou=People,dc=example,dc=org'
    expect(pageTitle(`/users/${encodeDN(dn)}`)).toBe('jdoe · Users · LDAP Warden')
  })

  it('unescapes an RDN value holding a comma', () => {
    const dn = 'cn=Doe\\, John,ou=Groups,dc=example,dc=org'
    expect(pageTitle(`/groups/${encodeDN(dn)}`)).toBe('Doe, John · Groups · LDAP Warden')
  })

  it('names the create and import pages', () => {
    expect(pageTitle('/users/new')).toBe('New · Users · LDAP Warden')
    expect(pageTitle('/netgroups/import')).toBe('Import · Netgroups · LDAP Warden')
  })

  it('keeps the section when the segment is not a DN', () => {
    expect(pageTitle('/users/%%%')).toBe('Users · LDAP Warden')
  })
})
