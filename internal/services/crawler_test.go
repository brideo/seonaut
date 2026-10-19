package services_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stjudewashere/seonaut/internal/config"
	"github.com/stjudewashere/seonaut/internal/models"
	"github.com/stjudewashere/seonaut/internal/services"
)

// crawlerTestRepository is a minimal mock that counts SaveCrawl calls and
// records what a resumed crawl does.
type crawlerTestRepository struct {
	lock           sync.Mutex
	saveCrawlCount int
	lastCrawl      models.Crawl
	crawledURLs    []string
	frontier       []string
	recounted      int64
	updated        *models.Crawl
	deletedExcept  int64
}

func (r *crawlerTestRepository) SaveCrawl(p models.Project) (*models.Crawl, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.saveCrawlCount++
	return &models.Crawl{Id: 1, ProjectId: p.Id, URL: p.URL}, nil
}

func (r *crawlerTestRepository) GetLastCrawl(p *models.Project) models.Crawl {
	return r.lastCrawl
}

func (r *crawlerTestRepository) GetLastFinishedCrawl(p *models.Project, beforeId int64) models.Crawl {
	return models.Crawl{}
}

func (r *crawlerTestRepository) DeleteUnfinishedProjectCrawls(p *models.Project, exceptId int64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.deletedExcept = exceptId
}

func (r *crawlerTestRepository) TouchCrawl(crawlId int64) {}

func (r *crawlerTestRepository) MarkCrawlEnded(c *models.Crawl) {}

func (r *crawlerTestRepository) RecountCrawl(c *models.Crawl) {
	r.recounted = c.Id
	c.TotalURLs = len(r.crawledURLs)
}

func (r *crawlerTestRepository) CrawledURLs(crawlId int64) []string {
	return r.crawledURLs
}

func (r *crawlerTestRepository) FrontierURLs(crawlId int64, followNofollow, includeExternal bool, f func(u string, depth int, resource bool)) {
	for _, u := range r.frontier {
		f(u, 1, false)
	}
}

func (r *crawlerTestRepository) GetLastCrawls(p models.Project, limit int) []models.Crawl {
	return []models.Crawl{}
}

func (r *crawlerTestRepository) DeleteCrawlData(c *models.Crawl) {}

func (r *crawlerTestRepository) CountIssuesByPriority(crawlId int64, priority int) int {
	return 0
}

func (r *crawlerTestRepository) UpdateCrawl(c *models.Crawl) {
	r.lock.Lock()
	defer r.lock.Unlock()

	updated := *c
	r.updated = &updated
}

// waitForUpdate waits for the crawl goroutine to save the finished crawl.
func (r *crawlerTestRepository) waitForUpdate(t *testing.T) *models.Crawl {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		r.lock.Lock()
		updated := r.updated
		r.lock.Unlock()

		if updated != nil {
			return updated
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatal("the crawl never finished")
	return nil
}

type crawlerHandlerTestRepository struct{}

func (r *crawlerHandlerTestRepository) SavePageReport(pr *models.PageReport, crawlId int64) (*models.PageReport, error) {
	return pr, nil
}

type crawlerReportManagerTestRepository struct{}

func (r *crawlerReportManagerTestRepository) SaveIssues(issues <-chan *models.Issue) {
	for range issues {
	}
}

func newTestCrawlerService(repo *crawlerTestRepository) *services.CrawlerService {
	broker := services.NewPubSubBroker()
	reportManager := services.NewReportManager(&crawlerReportManagerTestRepository{})
	handler := services.NewCrawlerHandler(&crawlerHandlerTestRepository{}, broker, reportManager)

	return services.NewCrawlerService(repo, services.CrawlerServicesContainer{
		Broker:         broker,
		ReportManager:  reportManager,
		CrawlerHandler: handler,
		ArchiveService: services.NewArchiveService(""),
		Config:         &config.CrawlerConfig{Agent: "testbot"},
	})
}

// TestStartCrawlerNoDuplicateDBRecord verifies that when StartCrawler is called
// while a crawl is already in progress, it returns an error and does not write
// a second crawl record to the DB — preventing the orphaned NULL-end-timestamp
// bug that permanently blocks future crawls.
func TestStartCrawlerNoDuplicateDBRecord(t *testing.T) {
	repo := &crawlerTestRepository{}
	svc := newTestCrawlerService(repo)

	// localhost:1 refuses connections immediately, so the background goroutine
	// finishes quickly without making the test slow.
	p := models.Project{Id: 1, URL: "http://localhost:1"}

	crawl, err := svc.StartCrawler(p, models.BasicAuth{})
	if err != nil {
		t.Fatalf("first StartCrawler: unexpected error: %v", err)
	}
	if crawl == nil || crawl.Id == 0 {
		t.Fatalf("first StartCrawler: expected a crawl with an id, got %+v", crawl)
	}
	if svc.IsCrawling(models.Project{Id: 999}) {
		t.Fatal("IsCrawling: expected false for a project that is not being crawled")
	}

	// Second call while the first goroutine still holds the in-memory lock.
	_, err = svc.StartCrawler(p, models.BasicAuth{})
	if !errors.Is(err, services.ErrCrawlerAlreadyRunning) {
		t.Fatalf("second StartCrawler: expected ErrCrawlerAlreadyRunning, got %v", err)
	}

	if repo.saveCrawlCount != 1 {
		t.Errorf("SaveCrawl called %d time(s), want exactly 1", repo.saveCrawlCount)
	}
}

// TestResumeCrawlerContinuesUnfinishedCrawl verifies that resuming keeps the
// unfinished crawl's id instead of creating a new crawl, counts the URLs it
// already crawled, and finishes the crawl.
func TestResumeCrawlerContinuesUnfinishedCrawl(t *testing.T) {
	repo := &crawlerTestRepository{
		lastCrawl:   models.Crawl{Id: 5, Crawling: true},
		crawledURLs: []string{"http://localhost:1/", "http://localhost:1/a"},
		frontier:    []string{"http://localhost:1/a", "http://localhost:1/b", "mailto:someone@example.com"},
	}
	svc := newTestCrawlerService(repo)
	p := models.Project{Id: 1, URL: "http://localhost:1"}

	crawl, err := svc.ResumeCrawler(p, models.BasicAuth{})
	if err != nil {
		t.Fatalf("ResumeCrawler: unexpected error: %v", err)
	}
	if crawl.Id != 5 {
		t.Errorf("resumed crawl id %d, want 5", crawl.Id)
	}

	updated := repo.waitForUpdate(t)

	if repo.saveCrawlCount != 0 {
		t.Errorf("SaveCrawl called %d time(s), want 0", repo.saveCrawlCount)
	}
	if repo.recounted != 5 {
		t.Errorf("RecountCrawl called for crawl %d, want 5", repo.recounted)
	}
	if updated.Id != 5 || updated.IssuesEnd.IsZero() {
		t.Errorf("crawl 5 was not finished: %+v", updated)
	}
	// The two crawled URLs plus the one new URL, /b.
	if updated.TotalURLs != 3 {
		t.Errorf("total URLs %d, want 3", updated.TotalURLs)
	}

	repo.lock.Lock()
	deletedExcept := repo.deletedExcept
	repo.lock.Unlock()
	if deletedExcept != 5 {
		t.Errorf("unfinished crawls deleted except %d, want 5", deletedExcept)
	}
}

// TestResumeCrawlerNothingToResume verifies a finished crawl can't be resumed.
func TestResumeCrawlerNothingToResume(t *testing.T) {
	repo := &crawlerTestRepository{lastCrawl: models.Crawl{Id: 5, Crawling: false}}
	svc := newTestCrawlerService(repo)

	_, err := svc.ResumeCrawler(models.Project{Id: 1, URL: "http://localhost:1"}, models.BasicAuth{})
	if !errors.Is(err, services.ErrNothingToResume) {
		t.Fatalf("expected ErrNothingToResume, got %v", err)
	}

	repo.lastCrawl = models.Crawl{}
	_, err = svc.ResumeCrawler(models.Project{Id: 1, URL: "http://localhost:1"}, models.BasicAuth{})
	if !errors.Is(err, services.ErrNothingToResume) {
		t.Fatalf("no crawl: expected ErrNothingToResume, got %v", err)
	}
}
