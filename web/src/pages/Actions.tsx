import { useSearch } from 'wouter'
import { useNavigation } from '../lib/routing'
import DashboardActionsPanel from '../components/dashboard/DashboardActionsPanel'

export default function Actions() {
  const { projectId } = useNavigation()
  const rawSearch = useSearch()
  const urlType = new URLSearchParams(rawSearch).get('type')
  const initialType = urlType === 'oa' || urlType === 'escalation' ? urlType : 'all'

  return <DashboardActionsPanel projectId={projectId} initialType={initialType} />
}
