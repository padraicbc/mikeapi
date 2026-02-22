package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme/autocert"

	"github.com/padraicbc/mikeapi/config"
	"github.com/padraicbc/mikeapi/db"
	"github.com/padraicbc/mikeapi/handlers"
	applog "github.com/padraicbc/mikeapi/logger"
	mw "github.com/padraicbc/mikeapi/middleware"
)

//go:embed all:build/*
var embeddedFiles embed.FS

func main() {
	cfg := config.Load()
	logger, err := applog.New(cfg.Debug)
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)

	bdb := db.Setup(cfg)
	defer bdb.Close()

	if err := db.CreateTables(context.Background(), bdb); err != nil {
		logger.Fatal("create tables failed", zap.Error(err))
	}

	h := handlers.New(bdb, cfg.JWTKey())

	e := echo.New()
	e.Use(echomw.RequestLoggerWithConfig(echomw.RequestLoggerConfig{
		LogMethod: true,
		LogURI:    true,
		LogStatus: true,
		LogError:  true,
		LogValuesFunc: func(c echo.Context, v echomw.RequestLoggerValues) error {
			fields := []zap.Field{
				zap.Int("status", v.Status),
				zap.String("method", v.Method),
				zap.String("uri", v.URI),
			}
			if v.Error != nil {
				fields = append(fields, zap.Error(v.Error))
			}
			switch {
			case v.Status >= 500:
				logger.Error("http request", fields...)
			case v.Status >= 400:
				logger.Warn("http request", fields...)
			default:
				logger.Info("http request", fields...)
			}
			return nil
		},
	}))
	e.Use(echomw.Recover())
	e.Use(echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"*", "Authorization"},
		AllowCredentials: true,
	}))

	// Public
	e.POST("/rp/signin", h.Signin)

	// Protected – require valid JWT in Authorization header
	rp := e.Group("/rp", mw.JWT(cfg.JWTKey()))
	rp.GET("/dates", h.Dates)
	rp.GET("/courses", h.Courses)
	rp.POST("/courses", h.CreateCourse)
	rp.GET("/results", h.Results)
	rp.POST("/analysis-results-update", h.ResultsAnalysis)
	rp.GET("/amended", h.ResultsAmended)
	rp.POST("/update-amended", h.UpdateAmended)
	rp.GET("/pre-race", h.GetPreRace)
	rp.POST("/save-to-intermediary", h.SaveToIntermediary)
	rp.POST("/update-pre-race", h.UpdatePreRace)
	rp.GET("/results-post-race", h.ResultsPostRace)
	rp.POST("/save-to-res-post-race", h.SaveToResPostRace)
	rp.GET("/form", h.GetForm)
	rp.GET("/trainers", h.GetAllTrainers)
	rp.GET("/trainer-notes", h.GetTrainerText)
	rp.POST("/trainer-save", h.SaveTrainerText)

	// Strip the "build/" prefix so URLs work correctly
	subFS, err := fs.Sub(embeddedFiles, "build")
	if err != nil {
		logger.Fatal("open embedded build fs failed", zap.Error(err))
	}
	// Serve static files correctly using Echo's WrapHandler
	fileServer := http.FileServer(http.FS(subFS))
	e.GET("/*", func(c echo.Context) error {
		path := c.Request().URL.Path

		// If request is for a static file, serve it
		if strings.Contains(path, ".") { // Matches JS, CSS, images, etc.
			http.StripPrefix("/", fileServer).ServeHTTP(c.Response(), c.Request())
			return nil
		}
		// Otherwise, serve `index.html` for client-side routing (SPA fallback)
		indexFile, err := subFS.Open("index.html")

		if err != nil {
			return c.NoContent(http.StatusNotFound)
		}
		defer indexFile.Close()

		return c.Stream(http.StatusOK, "text/html", indexFile)
	})

	if cfg.Debug {
		logger.Info("starting server", zap.String("mode", "debug"), zap.String("addr", cfg.Port))
		if err := e.Start(cfg.Port); err != nil {
			logger.Fatal("server exited", zap.Error(err))
		}
		return
	}

	autoTLS := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(".cache"),
		HostPolicy: autocert.HostWhitelist(cfg.TLSDomains...),
	}

	s := &http.Server{
		Addr:         ":443",
		Handler:      e,
		TLSConfig:    autoTLS.TLSConfig(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	if err := s.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
		logger.Error("tls server exited", zap.Error(err))
		os.Exit(1)
	}
}
