package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, World!")
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, version)
}

func main() {
	// Define command-line flags
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	http.HandleFunc("/", helloHandler)
	http.HandleFunc("/version", versionHandler)
	fmt.Println("Starting server at :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
