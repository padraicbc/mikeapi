package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"golang.org/x/crypto/acme/autocert"

	"github.com/padraicbc/mikeapi/config"
	"github.com/padraicbc/mikeapi/db"
	"github.com/padraicbc/mikeapi/handlers"
	mw "github.com/padraicbc/mikeapi/middleware"
)

//go:generate  pnpm run --prefix ../../ui build
//go:generate  cp -r ../../ui/build .
//go:embed all:build/*
var embeddedFiles embed.FS

func main() {
	log.SetFlags(log.Llongfile)

	cfg := config.Load()

	bdb := db.Setup(cfg)
	defer bdb.Close()

	if err := db.CreateTables(context.Background(), bdb); err != nil {
		log.Fatal("create tables:", err)
	}

	h := handlers.New(bdb, cfg.JWTKey())

	e := echo.New()
	e.Use(echomw.RequestLoggerWithConfig(echomw.RequestLoggerConfig{
		LogMethod: true,
		LogURI:    true,
		LogStatus: true,
		LogError:  true,
		LogValuesFunc: func(c echo.Context, v echomw.RequestLoggerValues) error {
			log.Printf("[%d] %s %s err=%v", v.Status, v.Method, v.URI, v.Error)
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
		log.Fatal(err)
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
		log.Println("debug mode – listening on", cfg.Port)
		log.Fatal(e.Start(cfg.Port))
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
		log.Fatal(err)
	}
}
