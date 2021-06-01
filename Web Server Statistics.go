/*
File Name:  Web Server Statistics.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner
*/

package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func startStatisticsWebServer() {
	if len(config.WebListen) == 0 {
		return
	}

	router := mux.NewRouter()

	router.Use(middlewareDisableCache())
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

func middlewareDisableCache() func(http.Handler) http.Handler {
	return (func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// always add no cache directive, to prevent loading from cache
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

			next.ServeHTTP(w, r)
		})
	})
}
