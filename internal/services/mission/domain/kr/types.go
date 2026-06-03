package kr

type KeyResultID string

type KeyResultStatus string

const (
	KeyResultStatusDraft      KeyResultStatus = "draft"
	KeyResultStatusOpen       KeyResultStatus = "open"
	KeyResultStatusInProgress KeyResultStatus = "in_progress"
	KeyResultStatusCompleted  KeyResultStatus = "completed"
	KeyResultStatusDropped    KeyResultStatus = "dropped"
)

type MeasurementType string

const (
	MeasurementNumber     MeasurementType = "number"
	MeasurementPercentage MeasurementType = "percentage"
	MeasurementBoolean    MeasurementType = "boolean"
)

type MeasurementDirection string

const (
	DirectionIncrease MeasurementDirection = "increase"
	DirectionDecrease MeasurementDirection = "decrease"
)
