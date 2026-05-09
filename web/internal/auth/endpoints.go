package auth

import (
	"log/slog"
	"net/http"

	"github.com/nickbryan/httputil"

	"github.com/nickbryan/objectory/web/internal/api"
)

// Endpoints returns all auth-related HTTP routes.
func Endpoints(logger *slog.Logger, iamClient *api.IAMClient) httputil.EndpointGroup {
	return httputil.EndpointGroup{
		{
			Method:  http.MethodGet,
			Path:    "/",
			Handler: indexHandler(),
		},
		{
			Method:  http.MethodGet,
			Path:    "/partials/login-form",
			Handler: loginFormHandler(),
		},
		{
			Method:  http.MethodGet,
			Path:    "/partials/registration-form",
			Handler: registrationFormHandler(),
		},
		{
			Method:  http.MethodPost,
			Path:    "/actions/register",
			Handler: identityCreateHandler(logger, iamClient),
		},
		{
			Method:  http.MethodPost,
			Path:    "/actions/login",
			Handler: sessionCreateHandler(logger, iamClient),
		},
		{
			Method:  http.MethodPost,
			Path:    "/actions/logout",
			Handler: sessionDeleteHandler(),
		},
	}
}
