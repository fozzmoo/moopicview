package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "MoopicView is running")
	})

	port := ":8080"
	log.Printf("Starting MoopicView server on %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
