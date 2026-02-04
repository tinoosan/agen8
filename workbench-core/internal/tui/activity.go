package tui

import (
	"github.com/tinoosan/workbench-core/pkg/types"
)

type ActivityStatus = types.ActivityStatus

const (
	ActivityPending = types.ActivityPending
	ActivityOK      = types.ActivityOK
	ActivityError   = types.ActivityError
)

type Activity = types.Activity
