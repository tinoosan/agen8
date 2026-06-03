import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Play, Pause, CheckCircle2, Archive } from 'lucide-react'
import type { MissionView, MissionStatus } from '../../lib/types'

export interface MissionLifecycleActionsProps {
  mission: MissionView
  onStatusChange: (status: MissionStatus) => void
  isPending: boolean
}

/**
 * Shared lifecycle action bar for a mission.
 *
 * Renders exactly the transitions allowed by the state machine in
 * `pkg/types/mission.go`:
 *   draft     → Activate
 *   active    → Pause, Complete (confirm)
 *   paused    → Resume, Archive (confirm)
 *   completed → Archive (confirm)
 *   archived  → (no actions)
 *
 * Pure UI: owns no mutation state. Parent supplies `onStatusChange` and
 * `isPending` so the same component works from both `MissionEditor`
 * (Missions page) and `MissionPanel` (Strategy Map slide-over).
 */
export default function MissionLifecycleActions({
  mission,
  onStatusChange,
  isPending,
}: MissionLifecycleActionsProps) {
  const { status } = mission

  return (
    <div className="flex items-center gap-1.5 flex-wrap">
      {status === 'draft' && (
        <Button size="sm" variant="default" onClick={() => onStatusChange('active')} disabled={isPending}>
          <Play size={12} className="mr-1" />
          Activate
        </Button>
      )}
      {status === 'active' && (
        <>
          <Button size="sm" variant="secondary" onClick={() => onStatusChange('paused')} disabled={isPending}>
            <Pause size={12} className="mr-1" />
            Pause
          </Button>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button size="sm" variant="default" disabled={isPending}>
                <CheckCircle2 size={12} className="mr-1" />
                Complete
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Complete this mission?</AlertDialogTitle>
                <AlertDialogDescription>
                  This marks the mission as completed. You can still archive it later but cannot reactivate it.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={() => onStatusChange('completed')}>
                  Complete Mission
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </>
      )}
      {status === 'paused' && (
        <>
          <Button size="sm" variant="default" onClick={() => onStatusChange('active')} disabled={isPending}>
            <Play size={12} className="mr-1" />
            Resume
          </Button>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button size="sm" variant="secondary" disabled={isPending}>
                <Archive size={12} className="mr-1" />
                Archive
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Archive this mission?</AlertDialogTitle>
                <AlertDialogDescription>
                  Archived missions are hidden from active views. This action cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={() => onStatusChange('archived')}>
                  Archive Mission
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </>
      )}
      {status === 'completed' && (
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button size="sm" variant="secondary" disabled={isPending}>
              <Archive size={12} className="mr-1" />
              Archive
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Archive this mission?</AlertDialogTitle>
              <AlertDialogDescription>
                Archived missions are hidden from active views. This action cannot be undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction onClick={() => onStatusChange('archived')}>
                Archive Mission
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </div>
  )
}
