import { useEffect, useState } from 'react'
import { useDebounced } from './use-debounced'

/**
 * Keeps a search box and the URL's search term in sync.
 *
 * The input is local so typing stays immediate, while the URL — and therefore
 * the request — follows the debounced value. Values arriving from the URL (back
 * button, a pasted link, a cleared filter) are adopted during render rather
 * than from an effect, which is React's documented way to adjust state when an
 * input changes and avoids the extra render pass an effect would cost.
 */
export function useUrlSearchInput(
  urlValue: string,
  commit: (value: string) => void,
): [string, (value: string) => void] {
  const [input, setInput] = useState(urlValue)
  const [lastUrlValue, setLastUrlValue] = useState(urlValue)

  if (urlValue !== lastUrlValue) {
    setLastUrlValue(urlValue)
    setInput(urlValue)
  }

  const debounced = useDebounced(input)

  useEffect(() => {
    if (debounced !== urlValue) {
      commit(debounced)
    }
  }, [debounced, urlValue, commit])

  return [input, setInput]
}
