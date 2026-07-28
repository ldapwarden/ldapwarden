import * as React from "react"
import { cn } from "@/lib/utils"

interface TabsContextValue {
  value: string
  onValueChange: (value: string) => void
  // Base for the generated element ids that tie a tab to its panel.
  idPrefix: string
}

const TabsContext = React.createContext<TabsContextValue | null>(null)

function useTabs() {
  const context = React.useContext(TabsContext)
  if (!context) {
    throw new Error("Tabs components must be used within a Tabs provider")
  }
  return context
}

interface TabsProps extends React.HTMLAttributes<HTMLDivElement> {
  value: string
  onValueChange: (value: string) => void
  children: React.ReactNode
}

export function Tabs({ value, onValueChange, children, className, ...props }: TabsProps) {
  const idPrefix = React.useId()

  const context = React.useMemo(
    () => ({ value, onValueChange, idPrefix }),
    [value, onValueChange, idPrefix],
  )

  return (
    <TabsContext.Provider value={context}>
      <div className={cn("w-full", className)} {...props}>
        {children}
      </div>
    </TabsContext.Provider>
  )
}

interface TabsListProps extends React.HTMLAttributes<HTMLDivElement> {
  children: React.ReactNode
}

export function TabsList({ children, className, ...props }: TabsListProps) {
  return (
    <div
      className={cn(
        // max-w-full + horizontal scroll: with seven tabs the list is wider
        // than a phone screen, and an inline-flex row that cannot scroll drags
        // the whole document sideways instead.
        "inline-flex h-10 max-w-full items-center justify-start overflow-x-auto rounded-lg bg-muted p-1 text-muted-foreground",
        className
      )}
      role="tablist"
      {...props}
    >
      {children}
    </div>
  )
}

interface TabsTriggerProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  value: string
  children: React.ReactNode
}

export function TabsTrigger({ value, children, className, ...props }: TabsTriggerProps) {
  const { value: selectedValue, onValueChange, idPrefix } = useTabs()
  const isSelected = selectedValue === value

  // A tablist is a single tab stop: Tab moves past the whole set, and the
  // arrow keys move between tabs. Without this the only way to reach the last
  // tab from the keyboard is to Tab through every one before it.
  const handleKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    // Read the tab order from the DOM rather than tracking it in state: which
    // tabs render depends on the directory's object classes and the enabled
    // modules, and the DOM is the only place that always matches what the user
    // sees.
    const list = event.currentTarget.closest('[role="tablist"]')
    const tabs = Array.from(list?.querySelectorAll<HTMLButtonElement>('[role="tab"]') ?? [])
    const index = tabs.indexOf(event.currentTarget)
    let next: HTMLButtonElement | undefined

    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        next = tabs[(index + 1) % tabs.length]
        break
      case 'ArrowLeft':
      case 'ArrowUp':
        next = tabs[(index - 1 + tabs.length) % tabs.length]
        break
      case 'Home':
        next = tabs[0]
        break
      case 'End':
        next = tabs[tabs.length - 1]
        break
      default:
        return
    }

    event.preventDefault()
    if (next) {
      // The id encodes the tab value; see the `id` prop below.
      next.focus()
      next.click()
    }
  }

  return (
    <button
      type="button"
      role="tab"
      id={`${idPrefix}-tab-${value}`}
      aria-selected={isSelected}
      aria-controls={`${idPrefix}-panel-${value}`}
      tabIndex={isSelected ? 0 : -1}
      data-state={isSelected ? 'active' : 'inactive'}
      className={cn(
        "inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium ring-offset-background transition-all focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50",
        isSelected
          ? "bg-background text-foreground shadow-xs"
          : "hover:bg-background/50 hover:text-foreground",
        className
      )}
      onClick={() => onValueChange(value)}
      onKeyDown={handleKeyDown}
      {...props}
    >
      {children}
    </button>
  )
}

interface TabsContentProps extends React.HTMLAttributes<HTMLDivElement> {
  value: string
  children: React.ReactNode
}

export function TabsContent({ value, children, className, ...props }: TabsContentProps) {
  const { value: selectedValue, idPrefix } = useTabs()

  if (selectedValue !== value) {
    return null
  }

  return (
    <div
      role="tabpanel"
      id={`${idPrefix}-panel-${value}`}
      aria-labelledby={`${idPrefix}-tab-${value}`}
      tabIndex={0}
      className={cn("mt-4 ring-offset-background focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2", className)}
      {...props}
    >
      {children}
    </div>
  )
}
