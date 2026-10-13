package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"gopherdisk/internal/blobs"
	"gopherdisk/internal/data"
	"gopherdisk/internal/database"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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

func (app *Application) serve() error {
	srv := http.Server{
		Addr:         fmt.Sprintf(":%d", app.cfg.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	shutdownError := make(chan error)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		s := <-quit
		app.logger.Printf("shutdown server: %s", s.String())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		shutdownError <- srv.Shutdown(ctx)
	}()

	app.logger.Printf("starting GoperDisk server on http://127.0.0.1:%d", app.cfg.port)
	err := srv.ListenAndServe()

	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	// wait srv.Shutdown() done

	err = <-shutdownError

	if err == nil {
		app.logger.Print("stopped server")
	}

	return err
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

	go func() {
		user, err := storage.Users.New(context.Background(), "frametamer")
		if err != nil {
			app.logger.Printf("%v", err)
			return
		}

		movie, err := os.Open("internal/data/testdata/movie.mov")
		if err != nil {
			app.logger.Printf("%v", err)
			return
		}
		defer movie.Close()

		info, err := movie.Stat()
		if err != nil {
			app.logger.Printf("%v", err)
			return
		}

		file, err := storage.Files.UploadReader(
			context.Background(),
			user.ID,
			info.Name(),
			data.FileType("video/quicktime"),
			movie,
			info.Size(),
		)
		if err != nil {
			app.logger.Printf("%v", err)
			return
		}

		app.logger.Printf("File id: %s\n", file.ID)
	}()

	err = app.serve()

	if err != nil {
		logger.Fatal(err)
	}
}
