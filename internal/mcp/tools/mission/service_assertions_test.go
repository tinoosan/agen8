package mission

import missionapp "github.com/tinoosan/agen8-mcp-server/internal/services/mission/app"

var _ MissionLifecycleService = (*missionapp.Service)(nil)
var _ KeyResultService = (*missionapp.Service)(nil)
var _ ProgressService = (*missionapp.Service)(nil)
