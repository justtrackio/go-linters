package validation

import (
	"context"
	"unsafe"

	"github.com/justtrackio/gosoline/pkg/log"
)

type PointerError struct{}

func (*PointerError) Error() string { return "error" }

type PointerStringer struct{}

func (*PointerStringer) String() string { return "string" }

type NamedPointer *int

type PointerAlias = *int

func Pointers(ctx context.Context, logger log.Logger, format string, number *int, named NamedPointer, alias PointerAlias, unsafePointer unsafe.Pointer) {
	logger.Info(ctx, "%v", number)                     // want "must not log pointer argument"
	logger.Info(ctx, "%T", number)                     // want "must not log pointer argument"
	logger.Warn(ctx, "%s", &PointerStringer{})         // want "must not log pointer argument"
	logger.Error(ctx, "%w", &PointerError{})           // want "must not log pointer argument"
	logger.Error(ctx, "%s", &PointerError{})           // want "must not log pointer argument" "error arguments"
	logger.Debug(ctx, "%v", named)                     // want "must not log pointer argument"
	logger.Trace(ctx, "%v", alias)                     // want "must not log pointer argument"
	logger.Info(ctx, "%v", unsafePointer)              // want "must not log pointer argument"
	logger.Info(ctx, format, number)                   // want "must not log pointer argument"
	logger.Info(ctx, "no directives", number)          // want "must not log pointer argument" "no formatting directives"
	logger.Info(ctx, "%p", number)                     // want "must not log pointer argument" "must not use pointer formatting"
	logger.Info(ctx, "%p", []int{1})                   // want "must not use pointer formatting"
	logger.Info(ctx, "%s", number)                     // want "must not log pointer argument" "wrong type"
	logger.Info(ctx, format, []any{number}...)         // want "must not log pointer argument"
	logger.Info(ctx, format, []any{1: number}...)      // want "must not log pointer argument"
	log.Logger.Info(logger, ctx, "%v", number)         // want "must not log pointer argument"
	logger.WithChannel("test").Info(ctx, "%v", number) // want "must not log pointer argument"
	logger.Info(ctx, "%d", *number)
	logger.Info(ctx, "%v", nil)
	logger.Info(ctx, "%s", (&PointerStringer{}).String())
	logger.Error(ctx, "%s", (&PointerError{}).Error())
	var opaque any = number
	logger.Info(ctx, "%v", opaque)
	OtherLogger{}.Info(ctx, "%v", number)
}

func PointerParameters[T ~*int, M ~*int | ~int](ctx context.Context, logger log.Logger, pointer T, maybePointer M) {
	logger.Info(ctx, "%v", pointer)      // want "must not log pointer argument"
	logger.Info(ctx, "%v", maybePointer) // want "must not log pointer argument"
}

func NamedPointerParameter[T NamedPointer](ctx context.Context, logger log.Logger, value T) {
	logger.Info(ctx, "%v", value) // want "must not log pointer argument"
}

func NonPointerIntersection[T interface {
	~*int | ~int
	~int
}](ctx context.Context, logger log.Logger, value T) {
	logger.Info(ctx, "%d", value)
}
