package api

import (
	"net/http"
	"time"

	"github.com/stjudewashere/seonaut/internal/models"
)

type createProjectRequest struct {
	UserID             int    `json:"user_id"`
	URL                string `json:"url"`
	IgnoreRobotsTxt    bool   `json:"ignore_robotstxt"`
	FollowNofollow     bool   `json:"follow_nofollow"`
	IncludeNoindex     bool   `json:"include_noindex"`
	CrawlSitemap       bool   `json:"crawl_sitemap"`
	AllowSubdomains    bool   `json:"allow_subdomains"`
	CheckExternalLinks bool   `json:"check_external_links"`
	BasicAuth          bool   `json:"basic_auth"`
	Archive            bool   `json:"archive"`
	UserAgent          string `json:"user_agent"`
}

type projectResponse struct {
	ID                 int64     `json:"id"`
	UserID             int       `json:"user_id"`
	URL                string    `json:"url"`
	Host               string    `json:"host"`
	IgnoreRobotsTxt    bool      `json:"ignore_robotstxt"`
	FollowNofollow     bool      `json:"follow_nofollow"`
	IncludeNoindex     bool      `json:"include_noindex"`
	CrawlSitemap       bool      `json:"crawl_sitemap"`
	AllowSubdomains    bool      `json:"allow_subdomains"`
	CheckExternalLinks bool      `json:"check_external_links"`
	BasicAuth          bool      `json:"basic_auth"`
	Archive            bool      `json:"archive"`
	UserAgent          string    `json:"user_agent"`
	Created            time.Time `json:"created"`
}

func newProjectResponse(p models.Project) projectResponse {
	return projectResponse{
		ID:                 p.Id,
		UserID:             p.UserId,
		URL:                p.URL,
		Host:               p.Host,
		IgnoreRobotsTxt:    p.IgnoreRobotsTxt,
		FollowNofollow:     p.FollowNofollow,
		IncludeNoindex:     p.IncludeNoindex,
		CrawlSitemap:       p.CrawlSitemap,
		AllowSubdomains:    p.AllowSubdomains,
		CheckExternalLinks: p.CheckExternalLinks,
		BasicAuth:          p.BasicAuth,
		Archive:            p.Archive,
		UserAgent:          p.UserAgent,
		Created:            p.Created,
	}
}

// createProject handles POST /api/projects.
func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := decodeJSON(w, r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if req.UserID <= 0 {
		writeError(w, http.StatusBadRequest, codeValidation, "user_id is required")
		return
	}

	if req.URL == "" {
		writeError(w, http.StatusBadRequest, codeValidation, "url is required")
		return
	}

	userAgent := req.UserAgent
	if userAgent == "" {
		userAgent = h.DefaultUserAgent
	}

	project := &models.Project{
		URL:                req.URL,
		IgnoreRobotsTxt:    req.IgnoreRobotsTxt,
		FollowNofollow:     req.FollowNofollow,
		IncludeNoindex:     req.IncludeNoindex,
		CrawlSitemap:       req.CrawlSitemap,
		AllowSubdomains:    req.AllowSubdomains,
		CheckExternalLinks: req.CheckExternalLinks,
		BasicAuth:          req.BasicAuth,
		Archive:            req.Archive,
		UserAgent:          userAgent,
	}

	if err := h.Projects.SaveProject(project, req.UserID); err != nil {
		writeServiceError(w, err)
		return
	}

	// Re-read the row so the response reflects what was stored (e.g. created).
	saved, err := h.Projects.FindProjectByID(project.Id)
	if err != nil {
		saved = *project
	}

	writeJSON(w, http.StatusCreated, newProjectResponse(saved))
}
