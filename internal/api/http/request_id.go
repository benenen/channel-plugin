package httpapi

import (
	"context"
	stdhttp "net/http"

	"github.com/benenen/channel-plugin/internal/domain"
)

type requestIDContextKey struct{}

func RequestIDMiddleware() func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			requestID := domain.NewPrefixedID("req")
			ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}
