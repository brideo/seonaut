package services_test

import (
	"errors"
	"testing"

	"github.com/stjudewashere/seonaut/internal/config"
	"github.com/stjudewashere/seonaut/internal/models"
	"github.com/stjudewashere/seonaut/internal/services"
)

// crawlerTestRepository is a minimal mock that counts SaveCrawl calls.
type crawlerTestRepository struct {
	saveCrawlCount int
}

func (r *crawlerTestRepository) SaveCrawl(p models.Project) (*models.Crawl, error) {
	r.saveCrawlCount++
	return &models.Crawl{Id: 1, ProjectId: p.Id, URL: p.URL}, nil
}

func (r *crawlerTestRepository) GetLastCrawl(p *models.Project) models.Crawl {
	return models.Crawl{}
}

func (r *crawlerTestRepository) GetLastCrawls(p models.Project, limit int) []models.Crawl {
	return []models.Crawl{}
}

func (r *crawlerTestRepository) DeleteCrawlData(c *models.Crawl) {}

func (r *crawlerTestRepository) CountIssuesByPriority(crawlId int64, priority int) int {
	return 0
}

func (r *crawlerTestRepository) UpdateCrawl(c *models.Crawl) {}

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
