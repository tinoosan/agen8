import React from 'react'
import { rpcCall } from '../lib/rpc'
import { useQueryClient } from '@tanstack/react-query'
import ConfirmationDialog from './ConfirmationDialog'
import { Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export interface SpaceControlsProps {
  spaceId: string
  projectRoot: string | null
  className?: string
}

export default function SpaceControls({
  spaceId,
  projectRoot,
  className,
}: SpaceControlsProps) {
  const queryClient = useQueryClient()
  const [confirmDeleteOpen, setConfirmDeleteOpen] = React.useState(false)

  async function handleDialogConfirm() {
    setConfirmDeleteOpen(false)
    await rpcCall('space.delete', { spaceId, projectRoot: projectRoot ?? undefined })
    queryClient.invalidateQueries({ queryKey: ['project.space.list'] })
  }

  return (
    <>
      <div className={cn('inline-flex items-center gap-1', className)}>
        <Button
          variant="ghost-danger"
          size="icon"
          onClick={(e) => {
            e.stopPropagation()
            setConfirmDeleteOpen(true)
          }}
          aria-label="Delete space"
        >
          <Trash2 size={12} />
        </Button>
      </div>
      <ConfirmationDialog
        open={confirmDeleteOpen}
        title="Delete space"
        message="This removes the space and its member addresses."
        confirmLabel="Delete"
        tone="danger"
        onClose={() => setConfirmDeleteOpen(false)}
        onConfirm={() => { void handleDialogConfirm() }}
      />
    </>
  )
}
