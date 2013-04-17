package main

import (
	"log"
	"net/http"
)

func buyHandlerSuccess(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("mj5rLRjgu75mmVbVKBshfZAA4qktDkzKLZ"))
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "../samples/normal.html")
}

func main() {
	http.HandleFunc("/buy", buyHandlerSuccess)
	http.HandleFunc("/", rootHandler)

	log.Fatal(http.ListenAndServe(":9999", nil))
}
