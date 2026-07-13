package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"video-record/internal/httpapi"
)

func main() {
	server := &http.Server{
		Addr:              ":8080",
		Handler:           httpapi.NewRouter(httpapi.Dependencies{}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("video-record listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
