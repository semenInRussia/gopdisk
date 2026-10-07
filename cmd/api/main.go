package main

import (
	"flag"
	"fmt"
	"gopherdisk/internal/blobs"
	"gopherdisk/internal/database"
	"log"
	"net/http"
	"os"
	"time"
)

const version = "1.0.0"

type Config struct {
	port        int
	maxBlobSize int64
	db          struct {
		url string
	}
}

type Application struct {
	cfg    Config
	logger *log.Logger
}

func main() {
	var cfg Config

	flag.IntVar(&cfg.port, "port", 8080, "the port on which server will be located")
	flag.Int64Var(&cfg.maxBlobSize, "blob", blobs.BlobsMaxSize, "maximum size of one blob")
	flag.StringVar(&cfg.db.url, "db", database.DefaultURL(), "URL of database to use")

	flag.Parse()

	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime)

	var app = Application{
		cfg:    cfg,
		logger: logger,
	}

	srv := http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	logger.Printf("starting GoperDisk server on http://127.0.0.1:%d", cfg.port)
	err := srv.ListenAndServe()
	logger.Fatal(err)
}
