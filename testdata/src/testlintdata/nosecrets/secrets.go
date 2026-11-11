package nosecrets

import (
	"context"
	e "errors"
	f "fmt"

	"github.com/justtrackio/gosoline/pkg/log"
)

type request struct {
	Password string
	ApiKey   string
}

func check(ctx context.Context, logger log.Logger, password string, req request, err error) {
	_ = f.Errorf("rejected %s", password)          // want "confidential value"
	_ = e.New("rejected " + req.ApiKey)            // want "confidential value"
	logger.Error(ctx, "rejected %s", req.Password) // want "confidential value"
	logger.Info(ctx, "request %v", req)            // want "confidential value"
	logger.WithFields(log.Fields{"request": req}) // want "confidential value"
	logger.WithFields(log.Fields{"apiKey": "synthetic-key"}) // want "confidential value"
	logger.WithFields(log.Fields{"password": log.Secret(password), "error_code": "refresh_token"})
	alias := password
	_ = f.Errorf("alias %s", alias) // want "confidential value"
	makeError := f.Errorf
	_ = makeError("alias sink %s", req.ApiKey) // want "confidential value"
	writeError := logger.Error
	writeError(ctx, "alias logger %s", password)                 // want "confidential value"
	_ = e.New(f.Sprintf("nested %s", password))                  // want "confidential value"
	_ = f.Errorf("unprotected %s", string(log.Secret(password))) // want "confidential value"
	_ = f.Errorf("safe %s", log.Secret(password))
	logger.Error(ctx, "operation failed: %w", err)
	logger.Info(ctx, "provider error code: %s", "refresh_token")
	passwordLength := len(password)
	logger.Info(ctx, "password length %d", passwordLength)
}
