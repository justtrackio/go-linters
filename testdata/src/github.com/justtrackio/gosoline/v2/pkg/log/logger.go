package log

import "context"

type Logger interface {
	Trace(context.Context, string, ...any)
	Debug(context.Context, string, ...any)
	Info(context.Context, string, ...any)
	Warn(context.Context, string, ...any)
	Error(context.Context, string, ...any)
	WithChannel(string) Logger
}

type GosoLogger interface {
	Logger
	Close(context.Context) error
}

type Concrete struct{}

func NewLogger() *Concrete { return &Concrete{} }

func (*Concrete) Trace(context.Context, string, ...any) {}
func (*Concrete) Debug(context.Context, string, ...any) {}
func (*Concrete) Info(context.Context, string, ...any)  {}
func (*Concrete) Warn(context.Context, string, ...any)  {}
func (*Concrete) Error(context.Context, string, ...any) {}
func (l *Concrete) WithChannel(string) Logger           { return l }
