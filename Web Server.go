/*
File Name:  Web Server.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner
*/

package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

func startStatisticsWebServer() {
	if len(config.WebListen) == 0 {
		return
	}

	router := mux.NewRouter()

	router.Use(HeadersMiddleware(config.HTTPAccessAllow, config.UseSSL))

	router.HandleFunc("/stat/Daily Active Peers.csv", webStatDailyActive).Methods("GET")
	router.HandleFunc("/stat/daily.json", webStatDailyJSON).Methods("GET")
	router.HandleFunc("/stat/daily.json", CrossSiteOptionsResponse).Methods("OPTIONS")

	router.PathPrefix("/").Handler(http.FileServer(http.Dir(config.WebFiles))).Methods("GET")

	for _, listen := range config.WebListen {
		go startWebServer(listen, config.UseSSL, config.CertificateFile, config.CertificateKey, router, "Web Listen", parseDuration(config.HTTPTimeoutRead), parseDuration(config.HTTPTimeoutWrite))
	}
}

// startWebServer starts a web-server with given parameters and logs the status. If may block forever and only returns if there is an error.
// Info is optional and defines the name of the info variable to register the full web listen URL.
// The timeouts are intentionally required to force the caller to think about it as they can heavily depend on the use case (internal vs external).
func startWebServer(WebListen string, UseSSL bool, CertificateFile, CertificateKey string, Handler http.Handler, Info string, ReadTimeout, WriteTimeout time.Duration) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12} // for security reasons disable TLS 1.0/1.1

	server := &http.Server{
		Addr:         WebListen,
		Handler:      Handler,
		ReadTimeout:  ReadTimeout,  // ReadTimeout is the maximum duration for reading the entire request, including the body.
		WriteTimeout: WriteTimeout, // WriteTimeout is the maximum duration before timing out writes of the response. This includes processing time and is therefore the max time any HTTP function may take.
		//IdleTimeout:  IdleTimeout,  // IdleTimeout is the maximum amount of time to wait for the next request when keep-alives are enabled.
		TLSConfig: tlsConfig,
	}

	if UseSSL {
		// HTTPS
		if err := server.ListenAndServeTLS(CertificateFile, CertificateKey); err != nil {
			log.Printf("Error listening on '%s': %v\n", WebListen, err)
		}
	} else {
		// HTTP
		if err := server.ListenAndServe(); err != nil {
			log.Printf("Error listening on '%s': %v\n", WebListen, err)
		}
	}
}

// parseDuration is the same as time.ParseDuration without returning an error. Valid units are ms, s, m, h. For example "10s".
func parseDuration(input string) (result time.Duration) {
	result, _ = time.ParseDuration(input)
	return
}

// CrossSiteOptionsResponse handles OPTION requests. This may be required for both POST and GET requests when the requests are done via Ajax from a different domain.
func CrossSiteOptionsResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Max-Age", "86400")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
}

// HeadersMiddleware sets the CORS and HSTS headers according to the input. It returns a middleware function to be used with mux.Router.Use().
func HeadersMiddleware(HTTPAccessAllow string, SetHSTS bool) func(http.Handler) http.Handler {
	return (func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set CORS header
			// Previously this code only set it when r.Header.Get("Origin") != "", however this screwed up HTTP/2 requests where the internal PUSH_PROMISE ommitted the Origin header
			// and the client received the response then without the CORS header, resulting in refusal of the browser to show the content. Therefore, set it always.
			if HTTPAccessAllow != "" {
				w.Header().Set("Access-Control-Allow-Origin", HTTPAccessAllow)
			}

			// Set HSTS header. We include sub-domains to be secured. 1 year.
			if r.TLS != nil && SetHSTS {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	})
}

// CacheControlSetHeader sets the Cache-Control header. If AllowCaching is true it uses the setting CacheControlMaxAge (in seconds) for the response of max cache time.
// If AllowCaching is false it will set the max-age to 0.
func CacheControlSetHeader(w http.ResponseWriter, AllowCaching bool, CacheControlMaxAge int) {
	if AllowCaching {
		w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(CacheControlMaxAge))
	} else {
		// explanation to use max-age=0 over the no-cache directive:
		// via https://stackoverflow.com/questions/1046966/whats-the-difference-between-cache-control-max-age-0-and-no-cache
		// Old question now, but if anyone else comes across this through a search as I did, it appears that IE9 will be making use of this to configure the behaviour of resources when using the back and forward buttons. When max-age=0 is used, the browser will use the last version when viewing a resource on a back/forward press. If no-cache is used, the resource will be refetched.
		w.Header().Set("Cache-Control", "private, max-age=0")
	}
}
