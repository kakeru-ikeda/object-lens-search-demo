package middleware

import (
	"net/http"
	"net/url"
)

func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	allowAll := false
	for _, origin := range allowedOrigins {
		if origin == "*" {
			allowAll = true
		}
		allowed[origin] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if originAllowed(origin, allowed) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,X-Request-Id,Accept")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed map[string]struct{}) bool {
	if _, ok := allowed[origin]; ok {
		return true
	}
	alias := loopbackAlias(origin)
	if alias == "" {
		return false
	}
	_, ok := allowed[alias]
	return ok
}

func loopbackAlias(origin string) string {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	hostname := parsed.Hostname()
	if hostname != "localhost" && hostname != "127.0.0.1" {
		return ""
	}
	aliasHost := "localhost"
	if hostname == "localhost" {
		aliasHost = "127.0.0.1"
	}
	if port := parsed.Port(); port != "" {
		aliasHost += ":" + port
	}
	parsed.Host = aliasHost
	return parsed.String()
}
