package validation

import (
	"context"
	"fmt"

	"github.com/justtrackio/gosoline/pkg/log"
)

type Count int

type Label string

type StringValue struct{}

func (StringValue) String() string { return "value" }

type FormattedValue struct{}

func (FormattedValue) Format(fmt.State, rune) {}

func ValidTypes(ctx context.Context, logger log.Logger, err error, opaque any) {
	logger.Info(ctx, "%s %s %s", "text", []byte("bytes"), Label("label"))
	logger.Info(ctx, "%d %d %c %U %q", 42, Count(3), 'a', 'b', 65)
	logger.Info(ctx, "%t %f %g %x", true, 1.5, complex(1, 2), "hex")
	logger.Info(ctx, "%d %d", []int{1, 2}, map[int]int{1: 2})
	logger.Info(ctx, "%s %Z", StringValue{}, FormattedValue{})
	logger.Info(ctx, "%s", err)
	logger.Error(ctx, "%w", err)
	logger.Info(ctx, "%v %T", opaque, opaque)
	logger.Info(ctx, "%[2]*.[1]*[3]f", 2, 5, 1.25)
	logger.Info(ctx, "%% %s", "text")
	logger.Info(ctx, "%d", len("text"))
	logger.Info(ctx, "%s", fmt.Sprintf("%s", "nested"))
}

func Mismatches(ctx context.Context, logger log.Logger) {
	logger.Info(ctx, "%s", 42)                         // want "wrong type int"
	logger.Warn(ctx, "%d", "text")                     // want "wrong type string"
	logger.Error(ctx, "%t", "text")                    // want "wrong type string"
	logger.Debug(ctx, "%f", true)                      // want "wrong type bool"
	logger.Trace(ctx, "%w", "text")                    // want "wrong type string"
	logger.Error(ctx, "%w", 42)                        // want "wrong type int"
	logger.Info(ctx, "%d", []string{"text"})           // want "wrong type"
	logger.Info(ctx, "%d", map[int]string{1: "text"})  // want "wrong type"
	logger.Info(ctx, "%[2]d %[1]s", "text", true)      // want "wrong type bool"
	logger.Info(ctx, "%*s", "wide", "text")            // want "non-int"
	logger.Info(ctx, "%.*f", "precise", 1.5)           // want "non-int"
	logger.Info(ctx, "%[2]*[1]s", "text", "wide")      // want "non-int"
	logger.Info(ctx, "%s %d", "text")                  // want "reads arg #2"
	logger.Info(ctx, "%s", "text", 42)                 // want "needs 1 arg but has 2 args"
	logger.Info(ctx, "no directives", 42)              // want "no formatting directives"
	log.Logger.Info(logger, ctx, "%d", "text")         // want "wrong type string"
	logger.WithChannel("test").Info(ctx, "%d", "text") // want "wrong type string"
}

func Malformed(ctx context.Context, logger log.Logger, err error) {
	logger.Error(ctx, "trailing %", err)                    // want "missing verb"
	logger.Error(ctx, "%[0]v", err)                         // want "invalid argument index"
	logger.Error(ctx, "%[x]v", err)                         // want "invalid argument index"
	logger.Error(ctx, "%[1v", err)                          // want "missing closing"
	logger.Error(ctx, "%[9]v", err)                         // want "reads arg #9"
	logger.Error(ctx, "%[9223372036854775807]v %v %v", err) // want "invalid argument index"
	logger.Error(ctx, "%s")                                 // want "reads arg #1"
	logger.Info(ctx, "%z", 42)                              // want "unknown verb"
	logger.Info(ctx, "%#s", "text")                         // want "unrecognized flag"
}

func Expansions(ctx context.Context, logger log.Logger, args []any) {
	logger.Info(ctx, "%s %d", args...)
	logger.Info(ctx, "%[0]s", args...) // want "invalid argument index"
}

func GenericTypes[T ~int | ~string, N ~int](ctx context.Context, logger log.Logger, mixed T, number N) {
	logger.Info(ctx, "%d", mixed) // want "wrong type"
	logger.Info(ctx, "%d", number)
}

type OtherLogger struct{}

func (OtherLogger) Info(context.Context, string, ...any) {}

func Unrelated(ctx context.Context, logger OtherLogger) {
	logger.Info(ctx, "%s", 42)
}

func NestedFormatting(ctx context.Context, logger log.Logger) {
	logger.Info(ctx, "%d", "fmt.Errorf") // want "has arg .*fmt.Errorf.*wrong type string"
	// The standard printf linter owns mismatches in ordinary fmt calls.
	logger.Info(ctx, "%s", fmt.Sprintf("%s", 42))
	logger.Info(ctx, "%T", func(format string, args ...any) string {
		return fmt.Sprintf(format, args...)
	})
}
