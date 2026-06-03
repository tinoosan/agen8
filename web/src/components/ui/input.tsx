import * as React from "react"

import { cn } from "@/lib/utils"

const Input = React.forwardRef<HTMLInputElement, React.ComponentProps<"input">>(
  ({ className, type, ...props }, ref) => {
    return (
      <input
        type={type}
        className={cn(
          "flex h-9 w-full rounded-[var(--r-md)] border border-transparent bg-[var(--bg-elevated)] px-3 py-2 text-sm text-[var(--text-1)] placeholder:text-[var(--text-3)] transition-colors focus-visible:outline-none focus-visible:border-[var(--border-focus)] focus-visible:bg-[var(--bg-surface)] disabled:cursor-not-allowed disabled:opacity-50 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-[var(--text-1)]",
          className
        )}
        ref={ref}
        {...props}
      />
    )
  }
)
Input.displayName = "Input"

export { Input }
