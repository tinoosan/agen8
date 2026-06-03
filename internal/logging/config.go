package logging

const EnvLogLevel = "AGEN8_LOG_LEVEL"

type Config struct {
	// Level is one of debug, info, warn, or error. Empty defaults to info.
	Level string
}
