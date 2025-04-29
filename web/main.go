package main

import (
	"context"
	"embed"
	"log/slog"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"

	"github.com/nickbryan/objectory/web/internal/auth"
)

//go:embed views
var viewsFS embed.FS

func main() {
	ctx := context.Background()

	logger := slogutil.NewJSONLogger(slogutil.WithLevel(slog.LevelDebug))

	views, err := httputil.ReadHTMLTemplates(viewsFS)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to load views", slog.Any("error", err))
		return
	}

	codec := httputil.NewHTMXCodec(views)
	server := httputil.NewServer(logger, httputil.WithServerCodec(codec))

	server.Register(
		auth.Endpoints(logger, views)...,
	)

	server.Serve(ctx)
}
