package api_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stjudewashere/seonaut/internal/api"
	"github.com/stjudewashere/seonaut/internal/models"
	"github.com/stjudewashere/seonaut/internal/services"
)

const testKey = "secret-key"

// Fakes with function fields so each test can control behaviour.
type fakeUsers struct {
	SignUpFn func(email, password, lang, theme string) (*models.User, error)
}

func (f *fakeUsers) SignUp(email, password, lang, theme string) (*models.User, error) {
	return f.SignUpFn(email, password, lang, theme)
}

type fakeProjects struct {
	SaveFn func(p *models.Project, userId int) error
	FindFn func(id int64) (models.Project, error)
}

func (f *fakeProjects) SaveProject(p *models.Project, userId int) error { return f.SaveFn(p, userId) }
func (f *fakeProjects) FindProjectByID(id int64) (models.Project, error) {
	return f.FindFn(id)
}

type fakeCrawler struct {
	StartFn      func(p models.Project, b models.BasicAuth) (*models.Crawl, error)
	ResumeFn     func(p models.Project, b models.BasicAuth) (*models.Crawl, error)
	StopFn       func(p models.Project)
	IsCrawlingFn func(p models.Project) bool
}

func (f *fakeCrawler) StartCrawler(p models.Project, b models.BasicAuth) (*models.Crawl, error) {
	return f.StartFn(p, b)
}
func (f *fakeCrawler) ResumeCrawler(p models.Project, b models.BasicAuth) (*models.Crawl, error) {
	return f.ResumeFn(p, b)
}
func (f *fakeCrawler) StopCrawler(p models.Project)     { f.StopFn(p) }
func (f *fakeCrawler) IsCrawling(p models.Project) bool { return f.IsCrawlingFn(p) }

type fakePinger struct{ err error }

func (f *fakePinger) Ping() error { return f.err }

// newHandler builds a Handler with permissive defaults.
func newHandler() *api.Handler {
	return &api.Handler{
		Users: &fakeUsers{SignUpFn: func(email, password, lang, theme string) (*models.User, error) {
			return &models.User{Id: 7, Email: email, Password: "hash"}, nil
		}},
		Projects: &fakeProjects{
			SaveFn: func(p *models.Project, userId int) error {
				p.Id = 3
				p.UserId = userId
				p.Host = "example.com"
				return nil
			},
			FindFn: func(id int64) (models.Project, error) {
				if id != 3 {
					return models.Project{}, sql.ErrNoRows
				}
				return models.Project{Id: 3, UserId: 1, URL: "https://example.com", Host: "example.com", UserAgent: "ua"}, nil
			},
		},
		Crawler: &fakeCrawler{
			StartFn: func(p models.Project, b models.BasicAuth) (*models.Crawl, error) {
				return &models.Crawl{Id: 11, ProjectId: p.Id, URL: p.URL}, nil
			},
			ResumeFn: func(p models.Project, b models.BasicAuth) (*models.Crawl, error) {
				return &models.Crawl{Id: 10, ProjectId: p.Id, URL: p.URL}, nil
			},
			StopFn:       func(p models.Project) {},
			IsCrawlingFn: func(p models.Project) bool { return true },
		},
		Pinger:           &fakePinger{},
		DefaultUserAgent: "default-agent",
		APIKey:           testKey,
	}
}

func do(h *api.Handler, method, path, body string, headers map[string]string) (*httptest.ResponseRecorder, map[string]interface{}) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)

	var decoded map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &decoded)

	return rec, decoded
}

func auth() map[string]string { return map[string]string{"X-API-Key": testKey} }

func errCode(body map[string]interface{}) string {
	e, _ := body["error"].(map[string]interface{})
	c, _ := e["code"].(string)
	return c
}

func TestAuth(t *testing.T) {
	h := newHandler()

	tests := []struct {
		name    string
		path    string
		headers map[string]string
		want    int
	}{
		{"no key", "/api/users", nil, http.StatusUnauthorized},
		{"wrong key", "/api/users", map[string]string{"X-API-Key": "nope"}, http.StatusUnauthorized},
		{"wrong bearer", "/api/users", map[string]string{"Authorization": "Bearer nope"}, http.StatusUnauthorized},
		{"x-api-key", "/api/users", auth(), http.StatusCreated},
		{"bearer", "/api/users", map[string]string{"Authorization": "Bearer " + testKey}, http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := do(h, http.MethodPost, tt.path, `{"email":"a@b.c","password":"pw"}`, tt.headers)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want == http.StatusUnauthorized {
				if errCode(body) != "unauthorized" {
					t.Errorf("code %q, want unauthorized", errCode(body))
				}
				if rec.Header().Get("WWW-Authenticate") == "" {
					t.Error("missing WWW-Authenticate header")
				}
			}
		})
	}
}

func TestHealth(t *testing.T) {
	h := newHandler()

	rec, body := do(h, http.MethodGet, "/health", "", nil)
	if rec.Code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("health: %d %s", rec.Code, rec.Body.String())
	}

	h.Pinger = &fakePinger{err: errors.New("down")}
	rec, _ = do(h, http.MethodGet, "/health", "", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("health with db down: %d", rec.Code)
	}
}

func TestUnknownRoutesReturnJSON(t *testing.T) {
	h := newHandler()

	rec, body := do(h, http.MethodGet, "/nope", "", nil)
	if rec.Code != http.StatusNotFound || errCode(body) != "not_found" {
		t.Errorf("unknown route: %d %s", rec.Code, rec.Body.String())
	}

	rec, body = do(h, http.MethodGet, "/api/nope", "", auth())
	if rec.Code != http.StatusNotFound || errCode(body) != "not_found" {
		t.Errorf("unknown api route: %d %s", rec.Code, rec.Body.String())
	}

	rec, body = do(h, http.MethodGet, "/api/users", "", auth())
	if rec.Code != http.StatusMethodNotAllowed || errCode(body) != "method_not_allowed" {
		t.Errorf("wrong method: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreateUser(t *testing.T) {
	h := newHandler()

	rec, body := do(h, http.MethodPost, "/api/users", `{"email":"a@b.c","password":"pw"}`, auth())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if body["id"] != float64(7) || body["email"] != "a@b.c" {
		t.Errorf("unexpected body: %v", body)
	}
	if _, ok := body["password"]; ok {
		t.Error("response must not include the password")
	}

	rec, body = do(h, http.MethodPost, "/api/users", `{"email":`, auth())
	if rec.Code != http.StatusBadRequest || errCode(body) != "bad_request" {
		t.Errorf("malformed json: %d %s", rec.Code, rec.Body.String())
	}

	rec, body = do(h, http.MethodPost, "/api/users", `{"email":"a@b.c","password":"pw","bogus":1}`, auth())
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: %d %s", rec.Code, rec.Body.String())
	}

	errs := []struct {
		err  error
		want int
		code string
	}{
		{services.ErrInvalidEmail, http.StatusBadRequest, "validation_error"},
		{services.ErrInvalidPassword, http.StatusBadRequest, "validation_error"},
		{services.ErrUserExists, http.StatusConflict, "user_exists"},
		{errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	}
	for _, e := range errs {
		h.Users = &fakeUsers{SignUpFn: func(string, string, string, string) (*models.User, error) { return nil, e.err }}
		rec, body := do(h, http.MethodPost, "/api/users", `{"email":"a@b.c","password":"pw"}`, auth())
		if rec.Code != e.want || errCode(body) != e.code {
			t.Errorf("%v: got %d %q, want %d %q", e.err, rec.Code, errCode(body), e.want, e.code)
		}
	}
}

func TestCreateProject(t *testing.T) {
	h := newHandler()

	var saved *models.Project
	h.Projects.(*fakeProjects).SaveFn = func(p *models.Project, userId int) error {
		saved = p
		p.Id = 3
		p.UserId = userId
		return nil
	}

	rec, body := do(h, http.MethodPost, "/api/projects", `{"user_id":1,"url":"https://example.com"}`, auth())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if saved.UserAgent != "default-agent" {
		t.Errorf("default user agent not applied: %q", saved.UserAgent)
	}
	if saved.CrawlSitemap || saved.Archive || saved.BasicAuth {
		t.Errorf("flags should default to false: %+v", saved)
	}
	if body["id"] != float64(3) || body["host"] != "example.com" || body["user_id"] != float64(1) {
		t.Errorf("unexpected body: %v", body)
	}

	rec, _ = do(h, http.MethodPost, "/api/projects", `{"user_id":1,"url":"https://example.com","crawl_sitemap":true,"archive":true,"user_agent":"custom"}`, auth())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !saved.CrawlSitemap || !saved.Archive || saved.UserAgent != "custom" {
		t.Errorf("flags not passed through: %+v", saved)
	}

	bad := []string{
		`{"url":"https://example.com"}`,
		`{"user_id":0,"url":"https://example.com"}`,
		`{"user_id":1}`,
		`{"user_id":1,"url":"https://example.com","nope":true}`,
	}
	for _, b := range bad {
		rec, _ := do(h, http.MethodPost, "/api/projects", b, auth())
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", b, rec.Code)
		}
	}

	errs := []struct {
		err  error
		want int
		code string
	}{
		{services.ErrProtocolNotSupported, http.StatusBadRequest, "validation_error"},
		{services.ErrUserAgent, http.StatusBadRequest, "validation_error"},
		{errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	}
	for _, e := range errs {
		h.Projects.(*fakeProjects).SaveFn = func(*models.Project, int) error { return e.err }
		rec, body := do(h, http.MethodPost, "/api/projects", `{"user_id":1,"url":"https://example.com"}`, auth())
		if rec.Code != e.want || errCode(body) != e.code {
			t.Errorf("%v: got %d %q, want %d %q", e.err, rec.Code, errCode(body), e.want, e.code)
		}
	}
}

func TestStartCrawl(t *testing.T) {
	h := newHandler()

	rec, body := do(h, http.MethodPost, "/api/projects/3/crawl", "", auth())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if body["id"] != float64(11) || body["project_id"] != float64(3) || body["crawling"] != true {
		t.Errorf("unexpected body: %v", body)
	}

	rec, _ = do(h, http.MethodPost, "/api/projects/3/crawl", `{}`, auth())
	if rec.Code != http.StatusAccepted {
		t.Errorf("empty object body: %d", rec.Code)
	}

	rec, body = do(h, http.MethodPost, "/api/projects/999/crawl", "", auth())
	if rec.Code != http.StatusNotFound || errCode(body) != "not_found" {
		t.Errorf("missing project: %d %s", rec.Code, rec.Body.String())
	}

	rec, body = do(h, http.MethodPost, "/api/projects/abc/crawl", "", auth())
	if rec.Code != http.StatusBadRequest || errCode(body) != "bad_request" {
		t.Errorf("bad id: %d %s", rec.Code, rec.Body.String())
	}

	h.Crawler.(*fakeCrawler).StartFn = func(models.Project, models.BasicAuth) (*models.Crawl, error) {
		return nil, services.ErrCrawlerAlreadyRunning
	}
	rec, body = do(h, http.MethodPost, "/api/projects/3/crawl", "", auth())
	if rec.Code != http.StatusConflict || errCode(body) != "crawl_in_progress" {
		t.Errorf("already running: %d %s", rec.Code, rec.Body.String())
	}

	// Basic auth project without credentials.
	var gotAuth models.BasicAuth
	h.Crawler.(*fakeCrawler).StartFn = func(p models.Project, b models.BasicAuth) (*models.Crawl, error) {
		gotAuth = b
		return &models.Crawl{Id: 12, ProjectId: p.Id}, nil
	}
	h.Projects.(*fakeProjects).FindFn = func(id int64) (models.Project, error) {
		return models.Project{Id: id, URL: "https://example.com", BasicAuth: true}, nil
	}
	rec, body = do(h, http.MethodPost, "/api/projects/3/crawl", "", auth())
	if rec.Code != http.StatusBadRequest || errCode(body) != "validation_error" {
		t.Errorf("basic auth missing: %d %s", rec.Code, rec.Body.String())
	}
	rec, _ = do(h, http.MethodPost, "/api/projects/3/crawl", `{"auth_user":"u","auth_pass":"p"}`, auth())
	if rec.Code != http.StatusAccepted || gotAuth.AuthUser != "u" || gotAuth.AuthPass != "p" {
		t.Errorf("basic auth passed: %d %+v", rec.Code, gotAuth)
	}

	// Project being deleted.
	h.Projects.(*fakeProjects).FindFn = func(id int64) (models.Project, error) {
		return models.Project{Id: id, URL: "https://example.com", Deleting: true}, nil
	}
	rec, body = do(h, http.MethodPost, "/api/projects/3/crawl", "", auth())
	if rec.Code != http.StatusConflict || errCode(body) != "project_deleting" {
		t.Errorf("deleting project: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStopCrawl(t *testing.T) {
	h := newHandler()

	stopped := false
	h.Crawler.(*fakeCrawler).StopFn = func(models.Project) { stopped = true }

	rec, body := do(h, http.MethodPost, "/api/projects/3/crawl/stop", "", auth())
	if rec.Code != http.StatusAccepted || body["stopping"] != true || !stopped {
		t.Errorf("stop: %d %s stopped=%v", rec.Code, rec.Body.String(), stopped)
	}

	h.Crawler.(*fakeCrawler).IsCrawlingFn = func(models.Project) bool { return false }
	rec, body = do(h, http.MethodPost, "/api/projects/3/crawl/stop", "", auth())
	if rec.Code != http.StatusNotFound || errCode(body) != "not_found" {
		t.Errorf("stop with no crawl: %d %s", rec.Code, rec.Body.String())
	}

	rec, _ = do(h, http.MethodPost, "/api/projects/999/crawl/stop", "", auth())
	if rec.Code != http.StatusNotFound {
		t.Errorf("stop missing project: %d", rec.Code)
	}
}

func TestResumeCrawl(t *testing.T) {
	h := newHandler()

	rec, body := do(h, http.MethodPost, "/api/projects/3/crawl/resume", "", auth())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if body["id"] != float64(10) || body["project_id"] != float64(3) || body["crawling"] != true {
		t.Errorf("unexpected body: %v", body)
	}

	rec, _ = do(h, http.MethodPost, "/api/projects/999/crawl/resume", "", auth())
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing project: %d", rec.Code)
	}

	h.Crawler.(*fakeCrawler).ResumeFn = func(models.Project, models.BasicAuth) (*models.Crawl, error) {
		return nil, services.ErrNothingToResume
	}
	rec, body = do(h, http.MethodPost, "/api/projects/3/crawl/resume", "", auth())
	if rec.Code != http.StatusConflict || errCode(body) != "nothing_to_resume" {
		t.Errorf("nothing to resume: %d %s", rec.Code, rec.Body.String())
	}

	h.Crawler.(*fakeCrawler).ResumeFn = func(models.Project, models.BasicAuth) (*models.Crawl, error) {
		return nil, services.ErrCrawlerAlreadyRunning
	}
	rec, body = do(h, http.MethodPost, "/api/projects/3/crawl/resume", "", auth())
	if rec.Code != http.StatusConflict || errCode(body) != "crawl_in_progress" {
		t.Errorf("analysing issues: %d %s", rec.Code, rec.Body.String())
	}
}
