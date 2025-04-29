package auth

import (
	"html/template"
	"log/slog"
	"net/http"

	"github.com/nickbryan/httputil"
)

func Endpoints(logger *slog.Logger, views *template.Template) httputil.EndpointGroup {
	return httputil.EndpointGroup{
		{
			Method:  http.MethodGet,
			Path:    "/",
			Handler: indexHandler(logger, views),
		},
		{
			Method:  http.MethodGet,
			Path:    "/partials/login-form",
			Handler: loginFormHandler(logger, views),
		},
		{
			Method:  http.MethodGet,
			Path:    "/partials/registration-form",
			Handler: registrationFormHandler(logger, views),
		},
	}
}
