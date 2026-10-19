package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/stjudewashere/seonaut/internal/models"
)

type startCrawlRequest struct {
	AuthUser string `json:"auth_user"`
	AuthPass string `json:"auth_pass"`
}

type crawlResponse struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	URL       string    `json:"url"`
	Start     time.Time `json:"start"`
	Crawling  bool      `json:"crawling"`
}

type stopCrawlResponse struct {
	Stopping bool `json:"stopping"`
}

// loadProject parses the {id} path value and loads the project, writing the
// appropriate error response when it can't. ok is false when a response has
// already been written.
func (h *Handler) loadProject(w http.ResponseWriter, r *http.Request) (models.Project, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "invalid project id")
		return models.Project{}, false
	}

	project, err := h.Projects.FindProjectByID(id)
	if err != nil {
		writeServiceError(w, err)
		return models.Project{}, false
	}

	return project, true
}

// startCrawl handles POST /api/projects/{id}/crawl.
func (h *Handler) startCrawl(w http.ResponseWriter, r *http.Request) {
	project, ok := h.loadProject(w, r)
	if !ok {
		return
	}

	var req startCrawlRequest
	if err := decodeJSON(w, r, &req, true); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if project.Deleting {
		writeError(w, http.StatusConflict, codeProjectDeleting, "project is being deleted")
		return
	}

	if project.BasicAuth && req.AuthUser == "" {
		writeError(w, http.StatusBadRequest, codeValidation, "project requires basic auth: auth_user and auth_pass must be provided")
		return
	}

	crawl, err := h.Crawler.StartCrawler(project, models.BasicAuth{AuthUser: req.AuthUser, AuthPass: req.AuthPass})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, crawlResponse{
		ID:        crawl.Id,
		ProjectID: crawl.ProjectId,
		URL:       crawl.URL,
		Start:     crawl.Start,
		Crawling:  true,
	})
}

// resumeCrawl handles POST /api/projects/{id}/crawl/resume.
func (h *Handler) resumeCrawl(w http.ResponseWriter, r *http.Request) {
	project, ok := h.loadProject(w, r)
	if !ok {
		return
	}

	var req startCrawlRequest
	if err := decodeJSON(w, r, &req, true); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if project.Deleting {
		writeError(w, http.StatusConflict, codeProjectDeleting, "project is being deleted")
		return
	}

	if project.BasicAuth && req.AuthUser == "" {
		writeError(w, http.StatusBadRequest, codeValidation, "project requires basic auth: auth_user and auth_pass must be provided")
		return
	}

	crawl, err := h.Crawler.ResumeCrawler(project, models.BasicAuth{AuthUser: req.AuthUser, AuthPass: req.AuthPass})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, crawlResponse{
		ID:        crawl.Id,
		ProjectID: crawl.ProjectId,
		URL:       crawl.URL,
		Start:     crawl.Start,
		Crawling:  true,
	})
}

// stopCrawl handles POST /api/projects/{id}/crawl/stop.
func (h *Handler) stopCrawl(w http.ResponseWriter, r *http.Request) {
	project, ok := h.loadProject(w, r)
	if !ok {
		return
	}

	if !h.Crawler.IsCrawling(project) {
		writeError(w, http.StatusNotFound, codeNotFound, "project has no active crawl")
		return
	}

	h.Crawler.StopCrawler(project)

	writeJSON(w, http.StatusAccepted, stopCrawlResponse{Stopping: true})
}
