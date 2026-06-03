package logging

const EnvLogLevel = "AGEN8_LOG_LEVEL"
const EnvLogFile = "AGEN8_LOG_FILE"

type Config struct {
	// Level is one of debug, info, warn, or error. Empty defaults to info.
	Level string
	// File optionally mirrors log output to this file.
	File string
}
