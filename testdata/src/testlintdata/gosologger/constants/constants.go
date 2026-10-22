package constants

import (
	"context"

	"github.com/justtrackio/gosoline/pkg/log"
)

const errorFormat = "failure: %s"

func Constants(ctx context.Context, logger log.Logger, err error) {
	logger.Error(ctx, errorFormat, err)      // want "error arguments"
	logger.Error(ctx, "failure: "+"%s", err) // want "error arguments"
	logger.Error(ctx, "failure: "+"%w", err)
}
