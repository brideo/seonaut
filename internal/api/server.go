package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/stjudewashere/seonaut/internal/services"
)

// Server is the HTTP server exposing the API.
type Server struct {
	httpServer *http.Server
}

// NewServer wires the container's services into the API handler.
func NewServer(c *services.Container) *Server {
	h := &Handler{
		Users:            c.UserService,
		Projects:         c.ProjectService,
		Crawler:          c.CrawlerService,
		Pinger:           c.DB(),
		DefaultUserAgent: c.Config.Crawler.Agent,
		APIKey:           c.Config.API.Key,
	}

	addr := fmt.Sprintf("%s:%d", c.Config.HTTPServer.Server, c.Config.HTTPServer.Port)

	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           h.Routes(),
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// ListenAndServe starts the server and blocks until it stops.
func (s *Server) ListenAndServe() error {
	log.Printf("Starting API server at %s ...\n", s.httpServer.Addr)

	return s.httpServer.ListenAndServe()
}
