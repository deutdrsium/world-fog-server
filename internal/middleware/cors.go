package middleware

import (
	"net/http"

	"github.com/go-chi/cors"
)

func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Session-ID"},
		ExposedHeaders:   []string{"X-Session-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	})
}
