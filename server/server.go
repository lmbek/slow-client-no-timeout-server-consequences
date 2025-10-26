package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		start := time.Now()
		n, err := io.Copy(io.Discard, r.Body) // read the whole body slowly as it arrives
		if err != nil {
			http.Error(w, "read error: "+err.Error(), http.StatusBadRequest)
			return
		}
		dur := time.Since(start)

		log.Printf("read %d bytes in %v\n", n, dur)
		fmt.Fprintf(w, "ok: read %d bytes in %v\n", n, dur)
	})

	srv := &http.Server{
		Addr:              "localhost:8080",
		Handler:           mux,
		ReadHeaderTimeout: 0, // intentionally no timeout security
		ReadTimeout:       0, // intentionally no timeout security
		WriteTimeout:      0,
		IdleTimeout:       0,
	}

	log.Println("listening on http://localhost:8080")
	log.Fatal(srv.ListenAndServe())
}
