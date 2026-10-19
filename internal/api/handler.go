// Package api exposes SEOnaut's user, project and crawl operations as a small
// JSON API protected by a shared API key.
package api

import (
	"net/http"

	"github.com/stjudewashere/seonaut/internal/models"
)

const (
	defaultLang  = "en"
	defaultTheme = "light"

	// maxBodyBytes limits the size of request bodies the API will read.
	maxBodyBytes = 1 << 20
)

type (
	// UserService creates users.
	UserService interface {
		SignUp(email, password, lang, theme string) (*models.User, error)
	}

	// ProjectService creates and looks up projects.
	ProjectService interface {
		SaveProject(p *models.Project, userId int) error
		FindProjectByID(id int64) (models.Project, error)
	}

	// CrawlerService starts, resumes and stops crawls.
	CrawlerService interface {
		StartCrawler(p models.Project, b models.BasicAuth) (*models.Crawl, error)
		ResumeCrawler(p models.Project, b models.BasicAuth) (*models.Crawl, error)
		StopCrawler(p models.Project)
		IsCrawling(p models.Project) bool
	}

	// Pinger reports whether a dependency (the database) is reachable.
	Pinger interface {
		Ping() error
	}

	// Handler holds the services the API endpoints depend on.
	Handler struct {
		Users            UserService
		Projects         ProjectService
		Crawler          CrawlerService
		Pinger           Pinger
		DefaultUserAgent string
		APIKey           string
	}
)

// Routes returns the http.Handler serving every API endpoint. The /health
// endpoint is public, everything under /api requires the API key.
func (h *Handler) Routes() http.Handler {
	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/users", h.createUser)
	protected.HandleFunc("POST /api/projects", h.createProject)
	protected.HandleFunc("POST /api/projects/{id}/crawl", h.startCrawl)
	protected.HandleFunc("POST /api/projects/{id}/crawl/stop", h.stopCrawl)
	protected.HandleFunc("POST /api/projects/{id}/crawl/resume", h.resumeCrawl)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("/api/", requireAPIKey(h.APIKey, protected))

	// jsonErrors turns the mux's plain text 404/405 responses into JSON.
	return jsonErrors(mux)
}
