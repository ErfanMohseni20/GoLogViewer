package viewer

import (
	"context"
	"net/http"
	"time"
)

// contextWithTimeout bounds a streaming request so a test cannot hang if the
// handler fails to notice the client going away.
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
