package gosologger

import (
	"context"
	"errors"
	"fmt"

	glog "github.com/justtrackio/gosoline/pkg/log"
)

func Levels(ctx context.Context, logger glog.Logger, err error) {
	logger.Error(ctx, "failed: %s", err)  // want "error arguments"
	logger.Warn(ctx, "failed: %w", err)   // want "error arguments"
	logger.Info(ctx, "failed: %v", err)   // want "error arguments"
	logger.Debug(ctx, "failed: %+v", err) // want "error arguments"
	logger.Trace(ctx, "failed: %q", err)  // want "error arguments"
	logger.Error(ctx, "failed: %w", err)
	logger.Warn(ctx, "failed: %s", err)
	logger.Info(ctx, "failed: %s", err)
	logger.Debug(ctx, "failed: %s", err)
	logger.Trace(ctx, "failed: %s", err)
	logger.Error(ctx, "no error: %s %v %d", "text", 42, 7)
	logger.Warn(ctx, "no error: %w", "text")
	logger.Error(ctx, "no arguments")
	logger.Error(ctx, "nil: %v", nil)
	fmt.Printf("%s", err)
}

func Formats(ctx context.Context, logger glog.Logger, err error) {
	logger.Error(ctx, "job %s: %v (%d), then %s", "job", err, 42, err) // want "error arguments"
	logger.Error(ctx, "%% %s: %[3]v %[2]s", "job", err, err)           // want "error arguments"
	logger.Error(ctx, "%[2]v %s", "job", err, err)                     // want "error arguments"
	logger.Error(ctx, "%*.*v %s", 5, 2, err, err)                      // want "error arguments"
	logger.Warn(ctx, "%[3]*.[2]*[1]w", err, 2, 5)                      // want "error arguments"
	logger.Error(ctx, "%10.2v %s", err, err)                           // want "error arguments"
	logger.Warn(ctx, "%[1]w %[1]v %[1]s", err)                         // want "error arguments"
	logger.Error(ctx, `raw: %s`, err)                                  // want "error arguments"
	logger.Error(ctx, "escaped:\n\t\"%s\"", err)                       // want "error arguments"
	logger.Error(ctx, ("parenthesized: %s"), err)                      // want "error arguments"
	logger.Error(ctx, "literal %%s %%w: %w", err)
	logger.Warn(ctx, "%*s %s", 4, "text", err)
	logger.Error(ctx, "%[2]*[1]w", err, 4)
	logger.Error(ctx, "trailing %", err)
	logger.Error(ctx, "%[0]v", err)
	logger.Error(ctx, "%[x]v", err)
	logger.Error(ctx, "%[1v", err)
	logger.Error(ctx, "%[9]v", err)
	logger.Error(ctx, "%[9223372036854775807]v %v %v", err)
	logger.Error(ctx, "%s")
}

func Receivers(ctx context.Context, logger glog.GosoLogger, err error) {
	logger.Error(ctx, "%v", err)                            // want "error arguments"
	logger.WithChannel("test").Warn(ctx, "%w", err)         // want "error arguments"
	glog.NewLogger().Error(ctx, "%s", err)                  // want "error arguments"
	glog.Logger.Error(logger, ctx, "%v", err)               // want "error arguments"
	(*glog.Concrete).Warn(glog.NewLogger(), ctx, "%w", err) // want "error arguments"
	(logger.Error)(ctx, "%s", err)                          // want "error arguments"
}

type WrappedLogger struct {
	glog.Logger
}

func Embedded(ctx context.Context, logger WrappedLogger, err error) {
	logger.Error(ctx, "%v", err) // want "error arguments"
}

type ConcreteError struct{}

func (*ConcreteError) Error() string { return "error" }

func ErrorTypes[T error](ctx context.Context, logger glog.Logger, generic T) {
	err := errors.New("error")
	logger.Error(ctx, "%s", err)              // want "error arguments"
	logger.Error(ctx, "%v", &ConcreteError{}) // want "error arguments"
	logger.Warn(ctx, "%w", generic)           // want "error arguments"
	var opaque any = err
	logger.Error(ctx, "%v", opaque)
}

type OtherLogger struct{}

func (OtherLogger) Error(context.Context, string, ...any) {}
func (OtherLogger) Warn(context.Context, string, ...any)  {}

func Unrelated(ctx context.Context, logger OtherLogger, err error) {
	logger.Error(ctx, "%s", err)
	logger.Warn(ctx, "%w", err)
}

func Unknown(ctx context.Context, logger glog.Logger, format string, args []any, err error) {
	logger.Error(ctx, format, err)
	logger.Error(ctx, "%s", args...)
	bound := logger.Error
	bound(ctx, "%s", err)
}
