// Alphabet for generated passwords: no 0/O/1/l/I, so a password that has to be
// read out or retyped from a screen does not turn into a support ticket.
const GENERATE_ALPHABET = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%^&*-_=+'
const GENERATE_LENGTH = 20

/**
 * Generates a password with crypto-grade randomness.
 *
 * Rejection sampling rather than `% alphabet.length`: the modulo of a uniform
 * byte over a 69-character alphabet is biased toward the first characters.
 */
export function generatePassword(length = GENERATE_LENGTH): string {
  const max = Math.floor(256 / GENERATE_ALPHABET.length) * GENERATE_ALPHABET.length
  const out: string[] = []
  const buffer = new Uint8Array(length * 2)

  while (out.length < length) {
    crypto.getRandomValues(buffer)
    for (const byte of buffer) {
      if (out.length === length) break
      if (byte < max) out.push(GENERATE_ALPHABET[byte % GENERATE_ALPHABET.length])
    }
  }
  return out.join('')
}
