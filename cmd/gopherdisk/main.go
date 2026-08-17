package main

import "gopherdisk/internal/storage"

func main() {
	s := storage.Storage{}
	id := storage.UserID("hey")
	s.Upload(id, "madk1d", "html", []byte("hello"))
}
