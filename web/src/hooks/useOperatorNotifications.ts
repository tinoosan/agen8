import { useEffect, useRef } from 'react'
import { onNotification } from '../lib/rpc'
import { toast } from 'sonner'

/**
 * F2/F3: Browser push notifications + in-app toast for new escalations and OA actions.
 * Subscribes to SSE event.append and fires notifications for high/critical items.
 * Deep-links navigate to the specific project dashboard with the item id.
 * Sound alerts use the Web Audio API when soundEnabled is true.
 */

function playNotificationSound() {
  try {
    const ctx = new (window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext)()
    const oscillator = ctx.createOscillator()
    const gainNode = ctx.createGain()
    oscillator.connect(gainNode)
    gainNode.connect(ctx.destination)
    oscillator.frequency.setValueAtTime(880, ctx.currentTime)
    oscillator.frequency.exponentialRampToValueAtTime(440, ctx.currentTime + 0.1)
    gainNode.gain.setValueAtTime(0.3, ctx.currentTime)
    gainNode.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.3)
    oscillator.start(ctx.currentTime)
    oscillator.stop(ctx.currentTime + 0.3)
  } catch {
    // browser doesn't support Web Audio API or autoplay blocked — silently ignore
  }
}

interface UseOperatorNotificationsOptions {
  enabled: boolean
  projectId: string | null
  soundEnabled: boolean
}

export function useOperatorNotifications({ enabled, projectId, soundEnabled }: UseOperatorNotificationsOptions) {
  const permissionRef = useRef<NotificationPermission>('default')

  // Request permission on mount if not already granted
  useEffect(() => {
    if (!enabled || typeof Notification === 'undefined') return
    permissionRef.current = Notification.permission
    if (Notification.permission === 'default') {
      Notification.requestPermission().then(p => { permissionRef.current = p })
    }
  }, [enabled])

  // Subscribe to SSE events
  useEffect(() => {
    if (!enabled) return

    const unsub = onNotification('event.append', (notif: Record<string, unknown>) => {
      const event = notif?.event as Record<string, unknown> | undefined
      if (!event) return
      const type = (event.type as string) ?? ''
      const data = event.data as Record<string, string> | undefined
      if (!data) return

      const isEscalation = type === 'escalation.created'
      const isOA = type === 'oa.created'
      const isMission = type === 'mission.created'
      if (!isEscalation && !isOA && !isMission) return

      const urgency = (data.urgency ?? '').toLowerCase()
      const title = data.title ?? (isEscalation ? 'New escalation' : isOA ? 'New operator action' : 'New mission')
      const category = data.category ?? ''
      const isUrgent = urgency === 'critical' || urgency === 'high'
      const id = data.escalationId ?? data.actionId ?? data.missionId ?? ''

      // Build deep-link URL
      let deepLink: string
      if (!projectId) {
        deepLink = '/dashboard'
      } else if (isEscalation) {
        deepLink = `/project/${projectId}/dashboard?escalation=${id}`
      } else if (isOA) {
        deepLink = `/project/${projectId}/actions/${id}`
      } else {
        // mission
        deepLink = `/project/${projectId}/missions/${id}`
      }

      // Play sound for urgent notifications
      if (isUrgent && soundEnabled) {
        playNotificationSound()
      }

      // In-app toast (always)
      toast(title, {
        description: `${isEscalation ? 'Escalation' : isOA ? 'Action' : 'Mission'} — ${category} (${urgency})`,
        duration: isUrgent ? 10000 : 5000,
        action: id ? {
          label: 'Open',
          onClick: () => {
            window.location.href = deepLink
          },
        } : undefined,
      })

      // Browser notification (high/critical only)
      if (isUrgent && permissionRef.current === 'granted' && typeof Notification !== 'undefined') {
        const n = new Notification(title, {
          body: `${urgency.toUpperCase()} ${category} ${isEscalation ? 'escalation' : isOA ? 'action' : 'mission'} requires attention`,
          icon: '/favicon.svg',
          tag: id, // Prevents duplicate notifications for same item
        })
        n.onclick = () => {
          window.focus()
          window.location.href = deepLink
          n.close()
        }
      }
    })

    return unsub
  }, [enabled, projectId, soundEnabled])
}
