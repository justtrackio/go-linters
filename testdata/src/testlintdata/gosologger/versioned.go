package gosologger

import (
	"context"

	"github.com/justtrackio/gosoline/v2/pkg/log"
)

func Versioned(ctx context.Context, logger log.Logger, err error) {
	logger.Error(ctx, "%v", err) // want "error arguments"
	logger.Warn(ctx, "%w", err)  // want "error arguments"
}
