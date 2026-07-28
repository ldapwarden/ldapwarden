import type { CSSProperties } from 'react'

// Categorical hues available to the `.tone` class (see index.css). Order is
// the rotation used when colouring an open-ended set of values, so a directory
// with three employee types always gets blue / emerald / violet.
export const TONES = [
  'blue',
  'emerald',
  'violet',
  'amber',
  'pink',
  'cyan',
  'lime',
  'rose',
  'indigo',
  'teal',
] as const

export type Tone = (typeof TONES)[number] | 'slate'

// toneStyle feeds a hue to the `.tone` class. The class itself resolves
// lightness from the active theme, which is why the tint has to be passed as a
// custom property rather than as a Tailwind colour utility.
export function toneStyle(tone: Tone): CSSProperties {
  return { '--tone': `var(--tone-${tone})` } as CSSProperties
}

// assignTones maps an arbitrary set of values onto the rotation above. Sorting
// first keeps a value's colour stable across renders and pages regardless of
// the order the directory returned it in.
export function assignTones(values: string[]): Map<string, Tone> {
  const unique = [...new Set(values)].sort()
  const map = new Map<string, Tone>()
  unique.forEach((value, index) => {
    map.set(value, TONES[index % TONES.length])
  })
  return map
}
