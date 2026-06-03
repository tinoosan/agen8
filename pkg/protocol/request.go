package protocol

// Request method names.
const (
	MethodSpaceCreate         = "space.create"
	MethodSpaceUpdate         = "space.update"
	MethodContextCompact      = "context.compact"
	MethodSpaceList           = "space.list"
	MethodSpaceGet            = "space.get"
	MethodChannelList         = "channel.list"
	MethodChannelGet          = "channel.get"
	MethodChannelSend         = "channel.send"
	MethodChannelMarkRead     = "channel.markRead"
	MethodMemberRegister      = "member.register"
	MethodMemberGet           = "member.get"
	MethodMemberList          = "member.list"
	MethodMemberUpdateConfig  = "member.updateConfig"
	MethodMemberRemove        = "member.remove"
	MethodSpaceClose          = "space.close"
	MethodSpaceReopen         = "space.reopen"
	MethodSpaceDelete         = "space.delete"
	MethodSpaceSend           = "space.send"
	MethodTurnCreate          = "turn.create"
	MethodTurnCancel          = "turn.cancel"
	MethodSpaceDetail         = "space.detail"
	MethodMessageList         = "message.list"
	MethodMessageGet          = "message.get"
	MethodMessageClaim        = "message.claim"
	MethodMessageHealth       = "message.health"
	MethodMessageRetry        = "message.retry"
	MethodTaskCreate          = "task.create"
	MethodTaskGet             = "task.get"
	MethodTaskList            = "task.list"
	MethodTaskUpdate          = "task.update"
	MethodSpaceRename         = "space.rename"
	MethodMemberRunList       = "member.run.list"
	MethodMemberRunStart      = "member.run.start"
	MethodSpaceStop           = "space.stop"
	MethodSpaceClearHistory   = "space.clearHistory"
	MethodSpaceGetTotals      = "space.getTotals"
	MethodSpaceGetStatus      = "space.getStatus"
	MethodSpaceGetManifest    = "space.getManifest"
	MethodSpaceGetRoster      = "space.getRoster"
	MethodPlanGet             = "plan.get"
	MethodPlanList            = "plan.list"
	MethodPlanAddPhase        = "plan.phase.add"
	MethodPlanReorderPhases   = "plan.phase.reorder"
	MethodPlanRemovePhase     = "plan.phase.remove"
	MethodPlanAddTodo         = "plan.todo.add"
	MethodPlanReorderTodos    = "plan.todo.reorder"
	MethodPlanRemoveTodo      = "plan.todo.remove"
	MethodPlanAddComment      = "plan.comment.add"
	MethodPlanMarkRead        = "plan.comment.markRead"
	MethodPlanAbandon         = "plan.abandon"
	MethodPlanSetKRRefs       = "plan.kr.set"
	MethodModelList           = "model.list"
	MethodModelRefresh        = "model.refresh"
	MethodControlSetModel     = "control.setModel"
	MethodControlSetReasoning = "control.setReasoning"
	MethodArtifactList        = "artifact.list"
	MethodArtifactSearch      = "artifact.search"
	MethodArtifactGet         = "artifact.get"
	MethodAttachmentUpload    = "attachment.upload"
	MethodAttachmentAddURL    = "attachment.addURL"
	MethodProjectGetContext   = "project.getContext"
	MethodProjectSetContext   = "project.setContext"
	MethodProjectListSpaces   = "project.listSpaces"
	MethodProjectGetSpace     = "project.getSpace"
	MethodProjectDeleteSpaces = "project.deleteSpaces"
	MethodLogsQuery           = "logs.query"
	MethodEventStream         = "event.stream"
	MethodFilesListDir        = "files.listDir"
	MethodFilesGet            = "files.get"
	MethodFilesCreateDir      = "files.createDir"
	MethodFilesCreateFile     = "files.createFile"
	MethodFilesMove           = "files.move"
	MethodFilesCopy           = "files.copy"
	MethodFilesDelete         = "files.delete"
	MethodFilesUpload         = "files.upload"
	MethodWorkspaceCreateDir  = "workspace.createDir"
	MethodWorkspaceCreateFile = "workspace.createFile"
	MethodWorkspaceMove       = "workspace.move"
	MethodWorkspaceDelete     = "workspace.delete"
	MethodWorkspaceUpload     = "workspace.upload"

	MethodProjectList   = "project.list"
	MethodProjectGet    = "project.get"
	MethodProjectCreate = "project.create"
	MethodProjectRemove = "project.remove"

	MethodEventsListPaginated = "events.listPaginated"
	MethodEventsLatestSeq     = "events.latestSeq"
	MethodEventsCount         = "events.count"
	MethodNotifyEventAppend   = "event.append"
	NotifyAgentOpStream       = "agent.op.stream"

	MethodMetricsProjectSummary = "metrics.projectSummary"
	MethodMetricsSpaceDetail    = "metrics.spaceDetail"
	MethodMetricsMemberDetail   = "metrics.memberDetail"
	MethodMetricsTimeSeries     = "metrics.timeSeries"

	MethodFSListDir = "fs.listDir"
)

// ArtifactListParams are the params for artifact.list.
type ArtifactListParams struct {
	Cwd         string  `json:"cwd,omitempty"`
	ProjectRoot string  `json:"projectRoot,omitempty"`
	SpaceID     SpaceID `json:"spaceId"`

	DayBucket string `json:"dayBucket,omitempty"`
	Role      string `json:"role,omitempty"`
	TaskKind  string `json:"taskKind,omitempty"`
	TaskID    string `json:"taskId,omitempty"`

	Limit int `json:"limit,omitempty"`
}

// ArtifactSearchParams are the params for artifact.search.
type ArtifactSearchParams struct {
	Cwd         string  `json:"cwd,omitempty"`
	ProjectRoot string  `json:"projectRoot,omitempty"`
	SpaceID     SpaceID `json:"spaceId"`
	Query       string  `json:"query"`
	ScopeKey    string  `json:"scopeKey,omitempty"`

	DayBucket string `json:"dayBucket,omitempty"`
	Role      string `json:"role,omitempty"`
	TaskKind  string `json:"taskKind,omitempty"`
	TaskID    string `json:"taskId,omitempty"`

	Limit int `json:"limit,omitempty"`
}

// ArtifactGetParams are the params for artifact.get.
type ArtifactGetParams struct {
	Cwd         string  `json:"cwd,omitempty"`
	ProjectRoot string  `json:"projectRoot,omitempty"`
	SpaceID     SpaceID `json:"spaceId"`
	ArtifactID  string  `json:"artifactId,omitempty"`
	VPath       string  `json:"vpath,omitempty"`
	MaxBytes    int     `json:"maxBytes,omitempty"`
}
