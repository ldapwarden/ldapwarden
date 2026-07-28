import { useEffect } from 'react'

// Typing in a field must never trigger a page-level shortcut: `/` belongs to the
// text you are writing, not to the search box.
function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.tagName === 'INPUT' ||
    target.tagName === 'TEXTAREA' ||
    target.tagName === 'SELECT' ||
    target.isContentEditable
  )
}

/**
 * Runs `handler` when the user presses Cmd+K / Ctrl+K, from anywhere including
 * inside a field — the palette is how you leave whatever you are doing.
 */
export function useCommandHotkey(handler: () => void) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key.toLowerCase() === 'k' && (event.metaKey || event.ctrlKey)) {
        event.preventDefault()
        handler()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [handler])
}

/**
 * Focuses a list's search box when the user presses `/`, the convention in
 * keyboard-driven tools. Ignored while typing, and while a modifier is held so
 * it cannot swallow browser shortcuts.
 */
export function useSearchHotkey(ref: React.RefObject<HTMLInputElement | null>) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return
      if (isTyping(event.target)) return
      event.preventDefault()
      ref.current?.focus()
      ref.current?.select()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [ref])
}
