package auth

import (
	"html/template"
	"log/slog"
	"net/http"

	"github.com/nickbryan/httputil"
)

func indexHandler(logger *slog.Logger, views *template.Template) http.HandlerFunc {
	const templateName = "index.html"

	return func(w http.ResponseWriter, r *http.Request) {
		if err := views.ExecuteTemplate(w, templateName, nil); err != nil {
			logger.ErrorContext(
				r.Context(),
				"Failed to execute view template",
				slog.Any("error", err),
				slog.String("template", templateName),
			)
		}
	}
}

func loginFormHandler(logger *slog.Logger, views *template.Template) http.HandlerFunc {
	const templateName = "forms/login.html"

	return func(w http.ResponseWriter, r *http.Request) {
		if err := views.ExecuteTemplate(w, templateName, nil); err != nil {
			logger.ErrorContext(
				r.Context(),
				"Failed to execute view template",
				slog.Any("error", err),
				slog.String("template", templateName),
			)
		}
	}
}

func loginHandler(logger *slog.Logger, views *template.Template) http.Handler {
	type request struct {
		Name                 string `form:"name" validate:"required,min=2,max=64"`
		Email                string `form:"email" validate:"required,email,max=255"`
		Password             string `form:"password" validate:"required,min=8,max=64"`
		PasswordConfirmation string `form:"passwordConfirmation" validate:"required,eqfield=Password"`
	}

	return httputil.NewHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
		return httputil.Redirect(http.StatusSeeOther, "/")
	})
}

func registrationFormHandler(logger *slog.Logger, views *template.Template) http.HandlerFunc {
	const templateName = "forms/register.html"

	return func(w http.ResponseWriter, r *http.Request) {
		if err := views.ExecuteTemplate(w, templateName, nil); err != nil {
			logger.ErrorContext(
				r.Context(),
				"Failed to execute view template",
				slog.Any("error", err),
				slog.String("template", templateName),
			)
		}
	}
}
