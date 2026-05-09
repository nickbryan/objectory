// Package dashboard provides the authenticated dashboard view.
package dashboard

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/nickbryan/httputil"

	"github.com/nickbryan/objectory/web/internal/api"
	"github.com/nickbryan/objectory/web/internal/auth"
)

func dashboardHandler(logger *slog.Logger, iamClient *api.IAMClient) http.Handler {
	type templateData struct {
		Name  string
		Email string
	}

	return httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
		token := auth.TokenFromContext(r.Context())

		identity, err := iamClient.GetMe(r.Context(), token)
		if err != nil {
			attrs := []any{slog.Any("error", err)}
			if respErr, ok := errors.AsType[*httputil.ProblemResponseError](err); ok {
				attrs = append(attrs, slog.Any("problem", respErr.Problem))
			}

			logger.WarnContext(r.Context(), "Failed to get identity for dashboard", attrs...)

			auth.ClearAuthCookie(r.ResponseWriter)

			return httputil.Redirect(http.StatusSeeOther, "/")
		}

		return httputil.OK(httputil.Template{
			Name: "pages/dashboard.html",
			Data: templateData{
				Name:  identity.Data.Name,
				Email: identity.Data.Email,
			},
		})
	})
}
