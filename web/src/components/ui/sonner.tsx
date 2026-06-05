import { Toaster as Sonner } from "sonner"

type ToasterProps = React.ComponentProps<typeof Sonner>

const Toaster = ({ ...props }: ToasterProps) => {
  return (
    <Sonner
      theme="dark"
      position="bottom-right"
      gap={8}
      toastOptions={{
        classNames: {
          toast:
            "rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-panel)] text-[var(--text-1)] shadow-[var(--shadow-lg)]",
          title: "text-[0.8125rem] font-medium text-[var(--text-1)]",
          description: "text-[0.75rem] text-[var(--text-2)]",
          actionButton:
            "rounded-[var(--r-md)] bg-[var(--accent)] px-2.5 py-1 text-[0.6875rem] font-medium text-white",
          cancelButton:
            "rounded-[var(--r-md)] bg-[var(--bg-elevated)] px-2.5 py-1 text-[0.6875rem] font-medium text-[var(--text-2)]",
          closeButton:
            "border border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text-2)]",
        },
        style: {
          background: "var(--bg-panel)",
          color: "var(--text-1)",
          borderColor: "var(--border)",
          boxShadow: "var(--shadow-lg)",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
