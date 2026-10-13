package services

import (
	"database/sql"
	"log"

	"github.com/stjudewashere/seonaut/internal/config"
	"github.com/stjudewashere/seonaut/internal/issues/multipage"
	"github.com/stjudewashere/seonaut/internal/issues/page"
	"github.com/stjudewashere/seonaut/internal/repository"

	_ "github.com/go-sql-driver/mysql"
)

type Container struct {
	Config         *config.Config
	PubSubBroker   *Broker
	ReportManager  *ReportManager
	UserService    *UserService
	ProjectService *ProjectService
	CrawlerService *CrawlerService
	ArchiveService *ArchiveService

	sqlDB                *sql.DB
	db                   *repository.Conn
	issueRepository      *repository.IssueRepository
	pageReportRepository *repository.PageReportRepository
	userRepository       *repository.UserRepository
	projectRepository    *repository.ProjectRepository
	crawlRepository      *repository.CrawlRepository
}

func NewContainer(configFile string) *Container {
	c := &Container{}
	c.InitConfig(configFile)
	c.InitDB()
	c.InitArchiveService()
	c.InitRepositories()
	c.InitPubSubBroker()
	c.InitReportManager()
	c.InitUserService()
	c.InitProjectService()
	c.InitCrawlerService()

	return c
}

// DB returns the underlying database handle, used for health checks.
func (c *Container) DB() *sql.DB {
	return c.sqlDB
}

// Load config file using the parameters in configFile.
func (c *Container) InitConfig(configFile string) {
	config, err := config.NewConfig(configFile)
	if err != nil {
		log.Fatalf("Error loading config: %v\n", err)
	}

	c.Config = config
}

// Create the sql database connection and run migrations.
func (c *Container) InitDB() {
	db, err := repository.SqlConnect(c.Config.DB)
	if err != nil {
		log.Fatalf("Error creating new database connection: %v", err)
	}

	if err := repository.RunMigrations(db, "migrations", c.Config.DB.TablePrefix); err != nil {
		log.Fatalf("Error running mysql migrations: %v", err)
	}

	c.sqlDB = db
	c.db = repository.NewConn(db, c.Config.DB.TablePrefix)
}

// Create the data repositories.
func (c *Container) InitRepositories() {
	c.issueRepository = &repository.IssueRepository{DB: c.db}
	c.pageReportRepository = &repository.PageReportRepository{DB: c.db}
	c.userRepository = &repository.UserRepository{DB: c.db}
	c.projectRepository = &repository.ProjectRepository{DB: c.db}
	c.crawlRepository = &repository.CrawlRepository{DB: c.db}

	// Clean up unfinished crawls.
	c.crawlRepository.DeleteUnfinishedCrawls()
}

// Create the PubSub broker.
func (c *Container) InitPubSubBroker() {
	c.PubSubBroker = NewPubSubBroker()
}

// Create the report manager and add all the available reporters.
func (c *Container) InitReportManager() {
	c.ReportManager = NewReportManager(c.issueRepository)
	for _, r := range page.GetAllReporters() {
		c.ReportManager.AddPageReporter(r)
	}

	// Create the sql multipage reporters and add them all to the reporterManager.
	sqlReporters := multipage.NewSqlReporter(c.db)
	for _, r := range sqlReporters.GetAllReporters() {
		c.ReportManager.AddMultipageReporter(r)
	}
}

// Create the user service.
func (c *Container) InitUserService() {
	c.UserService = NewUserService(c.userRepository)
}

// Create the Project service.
func (c *Container) InitProjectService() {
	repository := &struct {
		*repository.ProjectRepository
		*repository.CrawlRepository
	}{
		c.projectRepository,
		c.crawlRepository,
	}

	c.ProjectService = NewProjectService(repository, c.ArchiveService)

	// UserService DeleteHooks are called when a user is deleted.
	// Add a DeleteHook so it deletes all user projects and crawl
	// data when a user is deleted.
	c.UserService.AddDeleteHook(c.ProjectService.DeleteAllUserProjects)
}

// Create Crawler service.
func (c *Container) InitCrawlerService() {
	crawlerServices := CrawlerServicesContainer{
		Broker:         c.PubSubBroker,
		ReportManager:  c.ReportManager,
		CrawlerHandler: NewCrawlerHandler(c.pageReportRepository, c.PubSubBroker, c.ReportManager),
		ArchiveService: c.ArchiveService,
		Config:         c.Config.Crawler,
	}
	repository := &struct {
		*repository.CrawlRepository
		*repository.IssueRepository
	}{
		c.crawlRepository,
		c.issueRepository,
	}

	c.CrawlerService = NewCrawlerService(repository, crawlerServices)
}

// Init the WACZ archiver service.
func (c *Container) InitArchiveService() {
	c.ArchiveService = NewArchiveService("archive")
}
