// Package main is the entrypoint for the objectory web server.
package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"

	"github.com/nickbryan/objectory/web/internal/api"
	"github.com/nickbryan/objectory/web/internal/auth"
	"github.com/nickbryan/objectory/web/internal/dashboard"
)

func main() {
	ctx := context.Background()

	logger := slogutil.NewJSONLogger(slogutil.WithLevel(slog.LevelDebug))

	apiBaseURL := os.Getenv("API_BASE_URL")
	if apiBaseURL == "" {
		logger.ErrorContext(ctx, "API_BASE_URL environment variable is required")
		return
	}

	views, err := loadViews()
	if err != nil {
		logger.ErrorContext(ctx, "Failed to load views", slog.Any("error", err))
		return
	}

	staticFS, err := fs.Sub(viewsFS, "views/static")
	if err != nil {
		logger.ErrorContext(ctx, "Failed to create static filesystem", slog.Any("error", err))
		return
	}

	server := httputil.NewServer(logger, httputil.WithServerCodec(
		httputil.NewHTMLServerCodec(views,
			httputil.WithHTMLErrorTemplate(views.Lookup("pages/error.html")),
		),
	))

	server.Register(httputil.Endpoint{
		Method:  http.MethodGet,
		Path:    "/static/{path...}",
		Handler: http.StripPrefix("/static/", http.FileServerFS(staticFS)),
	})

	iamClient := api.NewIAMClient(logger, apiBaseURL)

	server.Register(auth.Endpoints(logger, iamClient)...)
	server.Register(dashboard.Endpoints(logger, iamClient)...)

	server.Serve(ctx)
}

//go:embed views
var viewsFS embed.FS

func loadViews() (*httputil.TemplateSet, error) {
	fsys, err := fs.Sub(viewsFS, "views")
	if err != nil {
		return nil, fmt.Errorf("creating sub filesystem: %w", err)
	}

	funcs := template.FuncMap{
		"fieldError": func(errors map[string]string, field string) string {
			if errors == nil {
				return ""
			}

			return errors[field]
		},
	}

	base := template.New("").Funcs(funcs)

	if err := parseDir(base, fsys, "layouts"); err != nil {
		return nil, fmt.Errorf("parsing layouts: %w", err)
	}

	if err := parseDir(base, fsys, "partials"); err != nil {
		return nil, fmt.Errorf("parsing partials: %w", err)
	}

	pages, err := readDir(fsys, "pages")
	if err != nil {
		return nil, fmt.Errorf("reading pages: %w", err)
	}

	ts, err := httputil.NewTemplateSet(base, pages)
	if err != nil {
		return nil, fmt.Errorf("creating template set: %w", err)
	}

	return ts, nil
}

// parseDir walks a directory within fsys and parses each .html file into the
// given template, using its path relative to fsys as the template name (e.g.
// "layouts/base.html").
func parseDir(t *template.Template, fsys fs.FS, dir string) error {
	if err := fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".html" {
			return err
		}

		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		if _, err := t.New(path).Parse(string(src)); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("walking %s: %w", dir, err)
	}

	return nil
}

// readDir reads each .html file in a directory and returns a map of name to
// source string, suitable for passing to [httputil.NewTemplateSet]. The name is
// the filename without its extension (e.g. "home" from "pages/home.html").
func readDir(fsys fs.FS, dir string) (map[string]string, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	pages := make(map[string]string, len(entries))

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".html" {
			continue
		}

		src, err := fs.ReadFile(fsys, filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}

		name := filepath.Join(dir, e.Name())
		pages[name] = string(src)
	}

	return pages, nil
}
