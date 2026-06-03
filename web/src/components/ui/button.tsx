import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-[var(--r-md)] font-medium font-[inherit] transition-[color,background,border-color] duration-150 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground hover:bg-primary/90",
        destructive:
          "bg-destructive text-destructive-foreground hover:bg-destructive/90",
        outline:
          "border border-[var(--border)] bg-transparent text-[var(--text-3)] text-xs hover:border-[var(--border-strong)] hover:text-[var(--text-2)]",
        secondary:
          "border border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text-2)] text-xs font-medium hover:border-[var(--border-strong)] hover:text-[var(--text-1)] hover:bg-[var(--bg-surface)]",
        ghost:
          "bg-transparent border-none text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)]",
        "ghost-danger":
          "bg-transparent border-none text-[var(--text-3)] hover:text-[var(--red)] hover:bg-[var(--red-dim)]",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "px-3 py-1.5 text-xs",
        sm: "px-2.5 py-1 text-xs",
        xs: "px-2 py-[3px] text-[10px] font-semibold rounded-[var(--r-sm)]",
        lg: "px-4 py-2 text-sm",
        icon: "p-[5px] [&_svg]:size-4",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button"
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    )
  }
)
Button.displayName = "Button"

// eslint-disable-next-line react-refresh/only-export-components
export { Button, buttonVariants }
