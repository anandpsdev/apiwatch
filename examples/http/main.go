package main

import (
	"log"
	"net/http"

	"github.com/anandpsdev/apiwatch"
	"github.com/anandpsdev/apiwatch/config"
)

func main() {
	cfg := config.Default()

	watch, err := apiwatch.New(cfg)
	if err != nil {
		log.Fatalf("failed to initialize apiwatch: %v", err)
	}
	defer watch.Close()

	mux := http.NewServeMux()

	// Mount dashboard handler at /apiwatch/
	mux.Handle("/apiwatch/", http.StripPrefix("/apiwatch", watch.Handler()))
	mux.Handle("/apiwatch", http.RedirectHandler("/apiwatch/", http.StatusMovedPermanently))

	// Application routes
	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"Hello from net/http"}`))
	})

	// Wrap entire mux with APIWatch middleware
	loggedHandler := watch.Middleware(mux)

	log.Println("APIWatch net/http demo listening on http://localhost:8081")
	log.Println("Open dashboard at http://localhost:8081/apiwatch/")
	if err := http.ListenAndServe(":8081", loggedHandler); err != nil {
		log.Fatal(err)
	}
}
