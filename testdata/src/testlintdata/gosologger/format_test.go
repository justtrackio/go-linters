package gosologger

import (
	"context"

	"github.com/justtrackio/gosoline/pkg/log"
)

func LogInTest(ctx context.Context, logger log.Logger, err error) {
	logger.Error(ctx, "%s", err) // want "error arguments"
	logger.Warn(ctx, "%w", err)  // want "error arguments"
}
