import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function encodeDN(dn: string): string {
  return btoa(dn).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function decodeDN(encoded: string): string {
  let base64 = encoded.replace(/-/g, '+').replace(/_/g, '/')
  while (base64.length % 4) {
    base64 += '='
  }
  return atob(base64)
}

// Helper function to parse LDAP generalized time to Date object
export function parseLdapTimestamp(timestamp: string): Date | null {
  if (!timestamp) return null
  const match = timestamp.match(/^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})/)
  if (!match) return null
  const [, year, month, day, hour, minute, second] = match
  return new Date(Date.UTC(
    parseInt(year), parseInt(month) - 1, parseInt(day),
    parseInt(hour), parseInt(minute), parseInt(second)
  ))
}

// Helper function to check if LDAP timestamp is in the future
export function isLdapTimestampInFuture(timestamp: string): boolean {
  const date = parseLdapTimestamp(timestamp)
  if (!date) return false
  return date > new Date()
}

// Helper function to format LDAP generalized time (e.g., "20240115143022Z")
export function formatLdapTimestamp(timestamp: string): string {
  const date = parseLdapTimestamp(timestamp)
  if (!date) return timestamp || 'N/A'
  return date.toLocaleString()
}

// Helper function to convert LDAP timestamp to date string (YYYY-MM-DD) for date picker
export function ldapTimestampToDateString(timestamp: string): string {
  const date = parseLdapTimestamp(timestamp)
  if (!date) return ''
  return date.toISOString().split('T')[0]
}
