package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/nickbryan/httputil"

	"github.com/nickbryan/objectory/web/internal/api"
)

const (
	authCookieMaxAge = int((24 * time.Hour) / time.Second) //nolint:mnd // 24 = hours per day.
)

// problemAttrs returns slog attrs for an API error, including the decoded
// problem details when the error is a *httputil.ProblemResponseError.
func problemAttrs(err error) []any {
	attrs := []any{slog.Any("error", err)}
	if respErr, ok := errors.AsType[*httputil.ProblemResponseError](err); ok {
		attrs = append(attrs, slog.Any("problem", respErr.Problem))
	}

	return attrs
}

func indexHandler() http.Handler {
	return httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
		if _, err := r.Cookie("auth_token"); err == nil {
			return httputil.Redirect(http.StatusSeeOther, "/dashboard")
		}

		return httputil.OK(httputil.Template{
			Name: "pages/index.html",
			Data: loginFormData{},
		})
	})
}

func loginFormHandler() http.Handler {
	return httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
		return httputil.OK(httputil.Template{
			Name: "partials/forms/login.html",
			Data: loginFormData{},
		})
	})
}

func registrationFormHandler() http.Handler {
	return httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
		return httputil.OK(httputil.Template{
			Name: "partials/forms/register.html",
			Data: registerFormData{},
		})
	})
}

type loginFormData struct {
	Email  string
	Error  string
	Errors map[string]string
}

type registerFormData struct {
	Name   string
	Email  string
	Error  string
	Errors map[string]string
}

func identityCreateHandler(logger *slog.Logger, iamClient *api.IAMClient) http.Handler {
	type request struct {
		Name                 string `form:"name"                 validate:"required,max=64"`
		Email                string `form:"email"                validate:"required,email"`
		Password             string `form:"password"             validate:"required,min=8,max=64,eqfield=PasswordConfirmation"`
		PasswordConfirmation string `form:"passwordConfirmation" validate:"required"`
	}

	return httputil.NewFormHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
		if r.Errors.HasAny() {
			return httputil.OK(httputil.Template{
				Name: "partials/forms/register.html",
				Data: registerFormData{
					Name:   r.Data.Name,
					Email:  r.Data.Email,
					Error:  "",
					Errors: r.Errors.All(),
				},
			})
		}

		_, err := iamClient.CreateIdentity(r.Context(), api.CreateIdentityRequest{
			Name:                 r.Data.Name,
			Email:                r.Data.Email,
			Password:             r.Data.Password,
			PasswordConfirmation: r.Data.PasswordConfirmation,
		})
		if err != nil {
			if errors.Is(err, api.ErrEmailAlreadyRegistered) {
				return httputil.OK(httputil.Template{
					Name: "partials/forms/register.html",
					Data: registerFormData{
						Name:   r.Data.Name,
						Email:  r.Data.Email,
						Error:  "",
						Errors: map[string]string{"email": "This email address is already registered."},
					},
				})
			}

			logger.ErrorContext(r.Context(), "Failed to create identity", problemAttrs(err)...)

			return httputil.OK(httputil.Template{
				Name: "partials/forms/register.html",
				Data: registerFormData{
					Name:   r.Data.Name,
					Email:  r.Data.Email,
					Error:  "An unexpected error occurred. Please try again.",
					Errors: nil,
				},
			})
		}

		return httputil.OK(httputil.Template{
			Name: "partials/forms/register-success.html",
			Data: nil,
		})
	})
}

func sessionCreateHandler(logger *slog.Logger, iamClient *api.IAMClient) http.Handler {
	type request struct {
		Email    string `form:"email"    validate:"required,email"`
		Password string `form:"password" validate:"required,min=8,max=64"`
	}

	return httputil.NewFormHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
		if r.Errors.HasAny() {
			return httputil.OK(httputil.Template{
				Name: "partials/forms/login.html",
				Data: loginFormData{
					Email:  r.Data.Email,
					Error:  "",
					Errors: r.Errors.All(),
				},
			})
		}

		tokenResp, err := iamClient.CreateToken(r.Context(), api.CreateTokenRequest{
			Email:    r.Data.Email,
			Password: r.Data.Password,
		})
		if err != nil {
			logger.ErrorContext(r.Context(), "Failed to create token", problemAttrs(err)...)

			return httputil.OK(httputil.Template{
				Name: "partials/forms/login.html",
				Data: loginFormData{
					Email:  r.Data.Email,
					Error:  "Invalid email or password.",
					Errors: nil,
				},
			})
		}

		setAuthCookie(r.ResponseWriter, tokenResp.Data.Token)

		r.ResponseWriter.Header().Set("HX-Redirect", "/dashboard")

		return httputil.NoContent()
	})
}

func sessionDeleteHandler() http.Handler {
	return httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
		ClearAuthCookie(r.ResponseWriter)

		r.ResponseWriter.Header().Set("HX-Redirect", "/")

		return httputil.NoContent()
	})
}

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		MaxAge:   authCookieMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearAuthCookie removes the auth_token cookie.
func ClearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
