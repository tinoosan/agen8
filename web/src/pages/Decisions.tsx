import { useNavigation } from '../lib/routing'
import DashboardDecisionsPanel from '../components/dashboard/DashboardDecisionsPanel'

// The standalone decision log is the full-page presentation of the shared
// DashboardDecisionsPanel (the same component the dashboard Decisions tab embeds).
export default function Decisions() {
  const { projectId } = useNavigation()
  return <DashboardDecisionsPanel projectId={projectId} />
}
