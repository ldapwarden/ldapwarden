import * as React from "react"
import { cn } from "@/lib/utils"
import { User } from "lucide-react"

interface AvatarProps extends React.HTMLAttributes<HTMLDivElement> {
  src?: string | null
  alt?: string
  fallback?: string
  size?: "sm" | "md" | "lg"
}

const sizeClasses = {
  sm: "h-8 w-8 text-xs",
  md: "h-10 w-10 text-sm",
  lg: "h-16 w-16 text-lg",
}

export function Avatar({
  src,
  alt,
  fallback,
  size = "md",
  className,
  ...props
}: AvatarProps) {
  const [hasError, setHasError] = React.useState(false)
  const [prevSrc, setPrevSrc] = React.useState(src)
  if (src !== prevSrc) {
    setPrevSrc(src)
    setHasError(false)
  }

  const initials = fallback
    ?.split(" ")
    .map((n) => n[0])
    .join("")
    .toUpperCase()
    .slice(0, 2)

  return (
    <div
      className={cn(
        "relative flex shrink-0 overflow-hidden rounded-full bg-muted",
        sizeClasses[size],
        className
      )}
      {...props}
    >
      {src && !hasError ? (
        <img
          src={`data:image/jpeg;base64,${src}`}
          alt={alt || fallback || "Avatar"}
          className="aspect-square h-full w-full object-cover"
          onError={() => setHasError(true)}
        />
      ) : initials ? (
        <span className="flex h-full w-full items-center justify-center bg-primary/10 text-primary font-medium">
          {initials}
        </span>
      ) : (
        <span className="flex h-full w-full items-center justify-center text-muted-foreground">
          <User className="h-1/2 w-1/2" />
        </span>
      )}
    </div>
  )
}
