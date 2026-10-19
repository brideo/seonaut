package services

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/stjudewashere/seonaut/internal/config"
	"github.com/stjudewashere/seonaut/internal/crawler"
	"github.com/stjudewashere/seonaut/internal/models"
)

const (
	LastCrawlsLimit   = 5                // Max number returned by GetLastCrawls
	ClientTimeout     = 10               // HTTP client timeout in seconds.
	HeartbeatInterval = 15 * time.Second // How often a running crawl records its last activity.
)

// ErrCrawlerAlreadyRunning is returned when a crawl is started for a project
// that is already being crawled.
var ErrCrawlerAlreadyRunning = errors.New("project is already being crawled")

// ErrNothingToResume is returned when a crawl is resumed for a project whose
// last crawl has finished.
var ErrNothingToResume = errors.New("project has no unfinished crawl to resume")

type CrawlerServiceRepository interface {
	SaveCrawl(models.Project) (*models.Crawl, error)
	GetLastCrawl(p *models.Project) models.Crawl
	GetLastCrawls(models.Project, int) []models.Crawl
	GetLastFinishedCrawl(p *models.Project, beforeId int64) models.Crawl
	DeleteCrawlData(c *models.Crawl)
	DeleteUnfinishedProjectCrawls(p *models.Project, exceptId int64)

	TouchCrawl(crawlId int64)
	MarkCrawlEnded(*models.Crawl)
	RecountCrawl(*models.Crawl)
	CrawledURLs(crawlId int64) []string
	FrontierURLs(crawlId int64, followNofollow, includeExternal bool, f func(u string, depth int, resource bool))

	CountIssuesByPriority(int64, int) int
	UpdateCrawl(*models.Crawl)
}

type CrawlerServicesContainer struct {
	Broker         *Broker
	ReportManager  *ReportManager
	CrawlerHandler *CrawlerHandler
	ArchiveService *ArchiveService
	Config         *config.CrawlerConfig
}

type CrawlerService struct {
	repository     CrawlerServiceRepository
	config         *config.CrawlerConfig
	broker         *Broker
	reportManager  *ReportManager
	crawlerHandler *CrawlerHandler
	ArchiveService *ArchiveService
	crawlers       map[int64]*crawler.Crawler
	analysing      map[int64]bool
	lock           *sync.RWMutex
}

func NewCrawlerService(r CrawlerServiceRepository, s CrawlerServicesContainer) *CrawlerService {
	return &CrawlerService{
		repository:     r,
		broker:         s.Broker,
		config:         s.Config,
		reportManager:  s.ReportManager,
		crawlerHandler: s.CrawlerHandler,
		ArchiveService: s.ArchiveService,
		crawlers:       make(map[int64]*crawler.Crawler),
		analysing:      make(map[int64]bool),
		lock:           &sync.RWMutex{},
	}
}

// StartCrawler creates a new crawler and crawls the project's URL.
// It adds a new crawler for the project, it returns an error if there's one already
// running or if there's an error creating it.
// Once the crawl finishes the previous crawl's data and any unfinished crawls are removed.
func (s *CrawlerService) StartCrawler(p models.Project, b models.BasicAuth) (*models.Crawl, error) {
	u, err := startURL(p)
	if err != nil {
		return nil, err
	}

	// Acquire the in-memory lock before any DB writes so that a rejected
	// duplicate trigger cannot leave an orphaned crawl record with a NULL
	// end timestamp.
	c, err := s.addCrawler(u, &p, &b)
	if err != nil {
		return nil, err
	}

	crawl, err := s.repository.SaveCrawl(p)
	if err != nil {
		s.removeCrawler(&p, c)
		return nil, err
	}
	crawl.Crawling = true

	previousCrawl := s.repository.GetLastFinishedCrawl(&p, crawl.Id)

	// Return a copy, taken before the crawl starts, so callers don't race
	// with the crawling goroutine.
	result := *crawl

	go s.run(crawl, p, c, previousCrawl, p.Archive, func() {
		log.Printf("Crawling %s...", p.URL)
		c.AddRequest(&crawler.RequestMessage{URL: u, Data: crawlerData{}})
	})

	return &result, nil
}

// ResumeCrawler continues the project's last crawl when it didn't finish, for instance
// because the crawler stopped responding or the server restarted. The URLs it already
// crawled are kept and skipped, and every URL they link to that hasn't been crawled
// is queued again. A crawler still registered for the project is replaced, unless it
// has finished crawling and is creating the crawl's issues.
func (s *CrawlerService) ResumeCrawler(p models.Project, b models.BasicAuth) (*models.Crawl, error) {
	u, err := startURL(p)
	if err != nil {
		return nil, err
	}

	last := s.repository.GetLastCrawl(&p)
	if last.Id == 0 || !last.Crawling {
		return nil, ErrNothingToResume
	}

	s.lock.Lock()
	if s.analysing[p.Id] {
		s.lock.Unlock()
		return nil, ErrCrawlerAlreadyRunning
	}
	if old, ok := s.crawlers[p.Id]; ok {
		log.Printf("Replacing the crawler of project %d to resume crawl %d", p.Id, last.Id)
		old.Supersede()
		delete(s.crawlers, p.Id)
	}
	s.lock.Unlock()

	c, err := s.addCrawler(u, &p, &b)
	if err != nil {
		return nil, err
	}

	crawl := &last
	crawl.ProjectId = p.Id
	crawl.URL = p.URL
	crawl.Crawling = true
	s.repository.RecountCrawl(crawl)
	s.repository.TouchCrawl(crawl.Id)

	previousCrawl := s.repository.GetLastFinishedCrawl(&p, crawl.Id)

	result := *crawl

	go s.run(crawl, p, c, previousCrawl, false, func() {
		seen := s.repository.CrawledURLs(crawl.Id)
		c.Seed(seen, len(seen))

		queued := 0
		s.repository.FrontierURLs(crawl.Id, p.FollowNofollow, p.AllowSubdomains, func(raw string, depth int, resource bool) {
			parsed, err := url.Parse(raw)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return
			}

			if c.AddRequest(&crawler.RequestMessage{URL: parsed, IgnoreDomain: resource, Data: crawlerData{Depth: depth}}) == nil {
				queued++
			}
		})

		log.Printf("Resuming crawl %d of %s: %d URLs already crawled, %d queued", crawl.Id, p.URL, len(seen), queued)
	})

	return &result, nil
}

// run crawls the project with the crawler, then creates the crawl's issues and saves it.
// queue is called before the crawl starts to add its first URLs. When the crawl finishes
// the previous crawl's data and the project's other unfinished crawls are deleted.
// A crawler that is superseded while it runs stops without finishing the crawl.
func (s *CrawlerService) run(crawl *models.Crawl, p models.Project, c *crawler.Crawler, previousCrawl models.Crawl, archive bool, queue func()) {
	defer s.removeCrawler(&p, c)

	callback := s.heartbeat(crawl, s.crawlerHandler.responseCallback(crawl, &p, c))

	if archive {
		archiver, err := s.ArchiveService.GetArchiveWriter(&p)
		if err != nil {
			log.Printf("Failed to create archive: %v", err)
		} else {
			defer archiver.Close()
			callback = s.crawlerHandler.archiveWrapper(callback, archiver)
		}
	}

	c.OnResponse(callback)

	queue()
	s.repository.TouchCrawl(crawl.Id)

	// Calling Start() initiates the website crawling process and
	// blocks execution until the crawling is complete.
	c.Start()

	if !s.startAnalysing(&p, c) {
		log.Printf("Crawl %d of %s was superseded", crawl.Id, p.URL)
		return
	}
	defer s.stopAnalysing(&p)

	crawl.RobotstxtExists = c.RobotstxtExists()
	crawl.SitemapExists = c.SitemapExists()
	crawl.SitemapIsBlocked = c.SitemapIsBlocked()
	crawl.End = time.Now()
	s.repository.MarkCrawlEnded(crawl)

	s.broker.Publish(fmt.Sprintf("crawl-%d", p.Id), &models.Message{Name: "IssuesInit"})
	s.reportManager.CreateMultipageIssues(crawl)

	crawl.IssuesEnd = time.Now()
	crawl.CriticalIssues = s.repository.CountIssuesByPriority(crawl.Id, Critical)
	crawl.AlertIssues = s.repository.CountIssuesByPriority(crawl.Id, Alert)
	crawl.WarningIssues = s.repository.CountIssuesByPriority(crawl.Id, Warning)
	crawl.TotalIssues = crawl.CriticalIssues + crawl.AlertIssues + crawl.WarningIssues

	s.repository.UpdateCrawl(crawl)
	s.broker.Publish(fmt.Sprintf("crawl-%d", p.Id), &models.Message{Name: "CrawlEnd", Data: crawl.TotalURLs})
	log.Printf("Crawled %d urls in %s", crawl.TotalURLs, p.URL)

	s.repository.DeleteUnfinishedProjectCrawls(&p, crawl.Id)
	if previousCrawl.Id != 0 {
		s.repository.DeleteCrawlData(&previousCrawl)
	}
}

// heartbeat wraps a response callback so the crawl records its last activity every
// HeartbeatInterval while it keeps receiving responses. A crawl that stops receiving
// them stops updating it, which is how a stalled crawl is spotted.
func (s *CrawlerService) heartbeat(crawl *models.Crawl, callback crawler.ResponseCallback) crawler.ResponseCallback {
	last := time.Now()

	return func(r *crawler.ResponseMessage) {
		callback(r)

		if time.Since(last) >= HeartbeatInterval {
			last = time.Now()
			s.repository.TouchCrawl(crawl.Id)
		}
	}
}

// startURL returns the project's URL to start crawling from.
func startURL(p models.Project) (*url.URL, error) {
	u, err := url.Parse(p.URL)
	if err != nil {
		return nil, err
	}

	if u.Path == "" {
		u.Path = "/"
	}

	return u, nil
}

// IsCrawling reports whether the project currently has a running crawler.
func (s *CrawlerService) IsCrawling(p models.Project) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.crawlers[p.Id]

	return ok
}

// Get a slice with 'LastCrawlsLimit' number of the crawls
func (s *CrawlerService) GetLastCrawls(p models.Project) []models.Crawl {
	crawls := s.repository.GetLastCrawls(p, LastCrawlsLimit)

	for len(crawls) < LastCrawlsLimit {
		crawls = append(crawls, models.Crawl{Start: time.Now()})
	}

	return crawls
}

// StopCrawler stops a crawler. If the crawler does not exsit it will just return.
func (s *CrawlerService) StopCrawler(p models.Project) {
	s.lock.Lock()
	defer s.lock.Unlock()

	crawler, ok := s.crawlers[p.Id]
	if !ok {
		return
	}

	crawler.Stop()
}

// AddCrawler creates a new project crawler and adds it to the crawlers map. It returns the crawler
// on success otherwise it returns an error indicating the crawler already exists or there was an
// error creating it.
func (s *CrawlerService) addCrawler(u *url.URL, p *models.Project, b *models.BasicAuth) (*crawler.Crawler, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.crawlers[p.Id]; ok {
		return nil, ErrCrawlerAlreadyRunning
	}

	limit := s.config.Limit
	if limit <= 0 {
		limit = config.DefaultCrawlLimit
	}

	timeoutHours := s.config.TimeoutHours
	if timeoutHours <= 0 {
		timeoutHours = config.DefaultCrawlTimeoutHours
	}

	options := &crawler.Options{
		CrawlLimit:      limit,
		Timeout:         time.Duration(timeoutHours) * time.Hour,
		IgnoreRobotsTxt: p.IgnoreRobotsTxt,
		FollowNofollow:  p.FollowNofollow,
		IncludeNoindex:  p.IncludeNoindex,
		CrawlSitemap:    p.CrawlSitemap,
		AllowSubdomains: p.AllowSubdomains,
	}

	mainDomain := strings.TrimPrefix(u.Host, "www.")

	httpClient := &http.Client{
		Timeout: ClientTimeout * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Make sure the user agent is not empty
	if p.UserAgent == "" {
		p.UserAgent = s.config.Agent
	}

	client := crawler.NewBasicClient(&crawler.ClientOptions{
		UserAgent:        p.UserAgent,
		BasicAuthDomains: []string{mainDomain, "www." + mainDomain},
		AuthUser:         b.AuthUser,
		AuthPass:         b.AuthPass,
	}, httpClient)

	// Creates a new crawler with the crawler's response handler.
	s.crawlers[p.Id] = crawler.NewCrawler(u, options, client)

	return s.crawlers[p.Id], nil
}

// RemoveCrawler removes a project's crawler from the crawlers map, unless it has
// already been replaced by another crawler.
func (s *CrawlerService) removeCrawler(p *models.Project, c *crawler.Crawler) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.crawlers[p.Id] == c {
		delete(s.crawlers, p.Id)
	}
}

// startAnalysing marks the project's crawl as creating its issues, so it can no longer
// be resumed. It returns false if the crawler has been superseded.
func (s *CrawlerService) startAnalysing(p *models.Project, c *crawler.Crawler) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	if c.Superseded() {
		return false
	}

	s.analysing[p.Id] = true

	return true
}

// stopAnalysing marks the project's crawl as done creating its issues.
func (s *CrawlerService) stopAnalysing(p *models.Project) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.analysing, p.Id)
}
