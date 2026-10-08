package main

import (
	"context"
	"flag"
	"fmt"
	"gopherdisk/internal/blobs"
	"gopherdisk/internal/data"
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
	storage     struct {
		dbURL string
		path  string
	}
}

type Application struct {
	cfg     Config
	storage *data.Storage
	logger  *log.Logger
}

func main() {
	var cfg Config

	flag.IntVar(&cfg.port, "port", 8080, "the port on which server will be located")
	flag.Int64Var(&cfg.maxBlobSize, "blob", blobs.BlobsMaxSize, "maximum size of one blob")
	flag.StringVar(&cfg.storage.dbURL, "db", database.DefaultURL(), "URL of database to use")
	flag.StringVar(&cfg.storage.path, "path", "data", "path to the directory where all located all data")

	flag.Parse()

	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime)

	db, err := database.Open(context.Background(), cfg.storage.dbURL)

	if err != nil {
		logger.Fatalf("cannot open database: %s", err.Error())
	}

	storage, err := data.MakeStorage(cfg.storage.path, db)

	if err != nil {
		logger.Fatalf("cannot open storage: %s", err.Error())
	}

	var app = Application{
		cfg:     cfg,
		logger:  logger,
		storage: storage,
	}

	srv := http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	logger.Printf("starting GoperDisk server on http://127.0.0.1:%d", cfg.port)
	err = srv.ListenAndServe()
	logger.Fatal(err)
}
