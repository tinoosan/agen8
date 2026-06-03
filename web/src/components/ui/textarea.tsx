import * as React from "react"

import { cn } from "@/lib/utils"

const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.ComponentProps<"textarea">
>(({ className, ...props }, ref) => {
  return (
    <textarea
      className={cn(
        // Mirrors the Input component focus style: subtle border color
        // change + bg shift, NO outer ring. The stock shadcn
        // `focus-visible:ring-2 ring-offset-2` pattern gave us a thick
        // blue ring around every textarea that read as visually loud
        // (user feedback: "this blue highlight on input areas needs to
        // be removed across the board").
        "flex min-h-[80px] w-full rounded-[var(--r-md)] border border-transparent bg-[var(--bg-elevated)] px-3 py-2 text-sm text-[var(--text-1)] placeholder:text-[var(--text-3)] transition-colors focus-visible:outline-none focus-visible:border-[var(--border-focus)] focus-visible:bg-[var(--bg-surface)] disabled:cursor-not-allowed disabled:opacity-50",
        className
      )}
      ref={ref}
      {...props}
    />
  )
})
Textarea.displayName = "Textarea"

export { Textarea }
