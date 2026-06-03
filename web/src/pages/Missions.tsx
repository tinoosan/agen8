import { useNavigation } from '../lib/routing'
import DashboardMissionsPanel from '../components/dashboard/DashboardMissionsPanel'

export default function Missions() {
  const { projectId, focusedProjectRoot } = useNavigation()
  return <DashboardMissionsPanel projectId={projectId} focusedProjectRoot={focusedProjectRoot} />
}
