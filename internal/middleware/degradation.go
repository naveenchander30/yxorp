package middleware

import (
	"context"
	"net/http"

	"github.com/yxorp/internal/degradation"
)

// DegradationMiddleware adds the degradation manager to the request context
func DegradationMiddleware(manager *degradation.Manager) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), "degradation_manager", manager)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
