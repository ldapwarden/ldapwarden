import * as React from "react"
import { ImageUp } from "lucide-react"
import { cn } from "@/lib/utils"

interface ImageDropTargetProps {
  // What the picture is, for screen readers ("Photo").
  label: string
  // Passed to the file picker. A dropped file bypasses it, so onFile still
  // has to check the type.
  accept: string
  onFile: (file: File) => void
  // The picture currently shown, or nothing: an empty target says how to fill it.
  children?: React.ReactNode
  disabled?: boolean
  className?: string
}

/**
 * ImageDropTarget makes the picture shown on a page the place to change it:
 * a click opens the file picker, a dropped file is handed over the same way,
 * and an empty one says so. It only picks the file; the form it sits in
 * decides when to save it.
 */
export function ImageDropTarget({
  label,
  accept,
  onFile,
  children,
  disabled,
  className,
}: ImageDropTargetProps) {
  const inputRef = React.useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = React.useState(false)

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    // Reset the input so picking the same file again (after an error) still
    // fires onChange.
    e.target.value = ""
    if (file) onFile(file)
  }

  const handleDragOver = (e: React.DragEvent) => {
    if (disabled) return
    e.preventDefault()
    e.dataTransfer.dropEffect = "copy"
    setDragging(true)
  }

  const handleDrop = (e: React.DragEvent) => {
    if (disabled) return
    e.preventDefault()
    setDragging(false)
    const file = e.dataTransfer.files[0]
    if (file) onFile(file)
  }

  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept={accept}
        onChange={handleChange}
        className="hidden"
        tabIndex={-1}
        aria-hidden="true"
      />
      <button
        type="button"
        aria-label={`${label}: drop an image or click to choose one`}
        disabled={disabled}
        data-dragging={dragging || undefined}
        onClick={() => inputRef.current?.click()}
        onDragOver={handleDragOver}
        onDragLeave={() => setDragging(false)}
        onDrop={handleDrop}
        className={cn(
          "group relative flex shrink-0 cursor-pointer items-center justify-center overflow-hidden border-2 border-dashed border-border transition-colors",
          "hover:border-primary focus-visible:border-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
          "data-[dragging]:border-primary data-[dragging]:bg-primary/10",
          "disabled:cursor-not-allowed disabled:opacity-50",
          className
        )}
      >
        {children ? (
          <>
            <span className="pointer-events-none h-full w-full">{children}</span>
            <span className="pointer-events-none absolute inset-x-0 bottom-0 bg-black/60 px-2 py-1 text-center text-xs font-medium text-white opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100 group-data-[dragging]:opacity-100">
              Drop or click
            </span>
          </>
        ) : (
          <span className="pointer-events-none flex flex-col items-center gap-1 px-2 text-center text-xs font-medium text-muted-foreground group-hover:text-primary group-data-[dragging]:text-primary">
            <ImageUp className="h-6 w-6" aria-hidden="true" />
            Drop an image or click
          </span>
        )}
      </button>
    </>
  )
}
