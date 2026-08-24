package logger

import "context"

type contextKey struct{}

// WithFields returns a context carrying log fields. Every entry written
// through a Logger's Ctx-aware methods picks them up, which is how a request
// ID reaches log lines written deep inside a call stack without being threaded
// through every function signature.
func WithFields(ctx context.Context, keysAndValues ...any) context.Context {
	fields := parseFields(keysAndValues...)
	if len(fields) == 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, mergeContext(fieldsFromContext(ctx), fields))
}

// Fields returns the log fields carried by ctx, or nil.
func Fields(ctx context.Context) map[string]any {
	return fieldsFromContext(ctx)
}

func fieldsFromContext(ctx context.Context) map[string]any {
	if ctx == nil {
		return nil
	}
	fields, _ := ctx.Value(contextKey{}).(map[string]any)
	return fields
}

// CtxLogger is a Logger bound to a context. Its methods mirror Logger's but
// automatically include the context's fields.
type CtxLogger struct {
	logger *Logger
	ctx    context.Context
}

// Ctx binds a context to this logger.
func (l *Logger) Ctx(ctx context.Context) *CtxLogger {
	return &CtxLogger{logger: l, ctx: ctx}
}

// With returns a context logger with additional bound fields.
func (c *CtxLogger) With(keysAndValues ...any) *CtxLogger {
	return &CtxLogger{logger: c.logger.With(keysAndValues...), ctx: c.ctx}
}

func (c *CtxLogger) Emergency(message string, keysAndValues ...any) error {
	return c.logger.log(c.ctx, LevelEmergency, message, nil, parseFields(keysAndValues...))
}

func (c *CtxLogger) Alert(message string, keysAndValues ...any) error {
	return c.logger.log(c.ctx, LevelAlert, message, nil, parseFields(keysAndValues...))
}

func (c *CtxLogger) Critical(message string, keysAndValues ...any) error {
	return c.logger.log(c.ctx, LevelCritical, message, nil, parseFields(keysAndValues...))
}

func (c *CtxLogger) Error(message string, err error, keysAndValues ...any) error {
	return c.logger.log(c.ctx, LevelError, message, err, parseFields(keysAndValues...))
}

func (c *CtxLogger) Warning(message string, keysAndValues ...any) error {
	return c.logger.log(c.ctx, LevelWarning, message, nil, parseFields(keysAndValues...))
}

func (c *CtxLogger) Notice(message string, keysAndValues ...any) error {
	return c.logger.log(c.ctx, LevelNotice, message, nil, parseFields(keysAndValues...))
}

func (c *CtxLogger) Info(message string, keysAndValues ...any) error {
	return c.logger.log(c.ctx, LevelInfo, message, nil, parseFields(keysAndValues...))
}

func (c *CtxLogger) Debug(message string, keysAndValues ...any) error {
	return c.logger.log(c.ctx, LevelDebug, message, nil, parseFields(keysAndValues...))
}

func (c *CtxLogger) Log(level Level, message string, keysAndValues ...any) error {
	return c.logger.log(c.ctx, level, message, nil, parseFields(keysAndValues...))
}
