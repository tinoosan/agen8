package app

type ConfigValidator interface {
	ValidateConfig(harnessKind, model, effort string) error
}

type runtimeConfigValidator interface {
	ValidateRuntimeConfig(harnessKind, model, effort, permissionMode, configRef string) error
}

type permissionModeDefaults interface {
	DefaultPermissionMode(harnessKind string) string
	CompatibilityPermissionMode(harnessKind string) string
}

type EventPublisher interface {
	Publish(topic string, event any) error
}
