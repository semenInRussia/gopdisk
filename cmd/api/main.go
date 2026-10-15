package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"gopherdisk/internal/blobs"
	"gopherdisk/internal/data"
	"gopherdisk/internal/database"
	"gopherdisk/internal/mailer"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const version = "1.0.0"

const (
	envSMTPHost     = "GOPHERDISK_SMTP_HOST"
	envSMTPPort     = "GOPHERDISK_SMTP_PORT"
	envSMTPUsername = "GOPHERDISK_SMTP_USERNAME"
	envSMTPPassword = "GOPHERDISK_SMTP_PASSWORD"
	envSMTPSender   = "GOPHERDISK_SMTP_SENDER"
)

type Config struct {
	port        int
	maxBlobSize int64
	storage     struct {
		dbURL string
		path  string
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
}

type mailSender interface {
	Send(recipient, templateFile string, data any) error
}

type Application struct {
	cfg     Config
	storage *data.Storage
	logger  *log.Logger
	mailer  mailSender
	wg      sync.WaitGroup
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	num, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return fallback
	}
	return int(num)
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
	app.wg.Wait()

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

	flag.StringVar(&cfg.smtp.host, "smtp-host", envOrDefault(envSMTPHost, "postbox.cloud.yandex.net"), "SMTP server hostname")
	flag.IntVar(&cfg.smtp.port, "smtp-port", envIntOrDefault(envSMTPPort, 587), "SMTP server port")
	flag.StringVar(&cfg.smtp.username, "smtp-username", envOrDefault(envSMTPUsername, ""), "SMTP username")
	flag.StringVar(&cfg.smtp.password, "smtp-password", envOrDefault(envSMTPPassword, ""), "SMTP password")
	flag.StringVar(&cfg.smtp.sender, "smtp-sender", envOrDefault(envSMTPSender, "GopherDisk <no-reply@gopherdisk.online>"), "SMTP sender")

	flag.Parse()

	blobs.BlobsMaxSize = cfg.maxBlobSize

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
		mailer:  mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender),
	}

	err = app.serve()

	if err != nil {
		logger.Fatal(err)
	}
}
