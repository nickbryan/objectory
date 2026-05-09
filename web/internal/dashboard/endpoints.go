package dashboard

import (
	"log/slog"
	"net/http"

	"github.com/nickbryan/httputil"

	"github.com/nickbryan/objectory/web/internal/api"
	"github.com/nickbryan/objectory/web/internal/auth"
)

// Endpoints returns the dashboard HTTP routes.
func Endpoints(logger *slog.Logger, iamClient *api.IAMClient) httputil.EndpointGroup {
	return httputil.EndpointGroup{
		{
			Method:  http.MethodGet,
			Path:    "/dashboard",
			Handler: dashboardHandler(logger, iamClient),
		},
	}.WithMiddleware(auth.RequireAuth)
}
