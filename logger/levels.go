package logger

type Level string

const (
	LevelEmergency Level = "emergency"
	LevelAlert     Level = "alert"
	LevelCritical  Level = "critical"
	LevelError     Level = "error"
	LevelWarning   Level = "warning"
	LevelNotice    Level = "notice"
	LevelInfo      Level = "info"
	LevelDebug     Level = "debug"
)

var levelPriority = map[Level]int{
	LevelEmergency: 800,
	LevelAlert:     700,
	LevelCritical:  600,
	LevelError:     500,
	LevelWarning:   400,
	LevelNotice:    300,
	LevelInfo:      200,
	LevelDebug:     100,
}

func ParseLevel(value string) Level {
	switch Level(value) {
	case LevelEmergency, LevelAlert, LevelCritical, LevelError,
		LevelWarning, LevelNotice, LevelInfo, LevelDebug:
		return Level(value)
	default:
		return LevelDebug
	}
}

func (l Level) Allows(min Level) bool {
	return levelPriority[l] >= levelPriority[min]
}

func AllLevels() []Level {
	return []Level{
		LevelEmergency,
		LevelAlert,
		LevelCritical,
		LevelError,
		LevelWarning,
		LevelNotice,
		LevelInfo,
		LevelDebug,
	}
}
