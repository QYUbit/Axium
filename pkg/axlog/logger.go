package axlog

type Logger interface {
	Info(s string, keyValues ...any)
	Error(s string, keyValues ...any)
	Debug(s string, keyValues ...any)
	Warn(s string, keyValues ...any)
}
