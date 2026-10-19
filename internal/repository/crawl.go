package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/stjudewashere/seonaut/internal/models"
)

type CrawlRepository struct {
	DB *Conn
}

// SaveCrawl inserts a new crawl into the database and returns a new Crawl model with
// the data provided by the project.
func (ds *CrawlRepository) SaveCrawl(p models.Project) (*models.Crawl, error) {
	stmt, _ := ds.DB.Prepare("INSERT INTO crawls (project_id, last_activity) VALUES (?, CURRENT_TIMESTAMP)")
	defer stmt.Close()
	res, err := stmt.Exec(p.Id)

	if err != nil {
		return nil, err
	}

	cid, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &models.Crawl{
		Id:        cid,
		ProjectId: p.Id,
		URL:       p.URL,
		Start:     time.Now(),
	}, nil
}

// GetLastCrawl returns a Crawl model with the last crawl stored for an specific project.
func (ds *CrawlRepository) GetLastCrawl(p *models.Project) models.Crawl {
	query := `
		SELECT
			id,
			start,
			end,
			total_urls,
			total_issues,
			critical_issues,
			alert_issues,
			warning_issues,
			issues_end,
			robotstxt_exists,
			sitemap_exists,
			sitemap_blocked,
			links_internal_follow,
			links_internal_nofollow,
			links_external_follow,
			links_external_nofollow,
			links_sponsored,
			links_ugc
		FROM crawls
		WHERE project_id = ?
		ORDER BY start DESC LIMIT 1`

	row := ds.DB.QueryRow(query, p.Id)

	var endTime, issuesEndTime sql.NullTime
	crawl := models.Crawl{Crawling: true}
	err := row.Scan(
		&crawl.Id,
		&crawl.Start,
		&endTime, // &crawl.End,
		&crawl.TotalURLs,
		&crawl.TotalIssues,
		&crawl.CriticalIssues,
		&crawl.AlertIssues,
		&crawl.WarningIssues,
		&issuesEndTime, // &crawl.IssuesEnd,
		&crawl.RobotstxtExists,
		&crawl.SitemapExists,
		&crawl.SitemapIsBlocked,
		&crawl.InternalFollowLinks,
		&crawl.InternalNoFollowLinks,
		&crawl.ExternalFollowLinks,
		&crawl.ExternalNoFollowLinks,
		&crawl.SponsoredLinks,
		&crawl.UGCLinks,
	)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("GetLastCrawl project id %d: %v\n", p.Id, err)
	}

	if endTime.Valid && issuesEndTime.Valid {
		crawl.End = endTime.Time
		crawl.IssuesEnd = issuesEndTime.Time
		crawl.Crawling = false
	}

	return crawl
}

// GetLastCrawls returns a slice with a number of crawls for the specific project. The number of crawls
// to be returned is specified with the limit parameter.
func (ds *CrawlRepository) GetLastCrawls(p models.Project, limit int) []models.Crawl {
	query := `
		SELECT
			id,
			start,
			end,
			total_urls,
			total_issues,
			issues_end,
			critical_issues,
			alert_issues,
			warning_issues,
			blocked_by_robotstxt,
			noindex
		FROM crawls
		WHERE project_id = ?
		ORDER BY start DESC LIMIT ?`

	crawls := []models.Crawl{}
	rows, err := ds.DB.Query(query, p.Id, limit)
	if err != nil {
		log.Println(err)
	}

	for rows.Next() {
		endTime := sql.NullTime{}
		issuesEndTime := sql.NullTime{}
		crawl := models.Crawl{Crawling: true}
		err := rows.Scan(
			&crawl.Id,
			&crawl.Start,
			&endTime, // &crawl.End,
			&crawl.TotalURLs,
			&crawl.TotalIssues,
			&issuesEndTime, // &crawl.IssuesEnd,
			&crawl.CriticalIssues,
			&crawl.AlertIssues,
			&crawl.WarningIssues,
			&crawl.BlockedByRobotstxt,
			&crawl.Noindex,
		)
		if err != nil {
			log.Printf("GetLastCrawl: %v\n", err)
		}
		if endTime.Valid && issuesEndTime.Valid {
			crawl.End = endTime.Time
			crawl.IssuesEnd = issuesEndTime.Time
			crawl.Crawling = false
		}
		crawls = append([]models.Crawl{crawl}, crawls...)
	}

	return crawls
}

// DeleteCrawlData deletes all the crawl's data in a batch process. It removes the crawl's associated
// links, external_links, hreflangs, issues, images and any other data associated to it.
func (ds *CrawlRepository) DeleteCrawlData(crawl *models.Crawl) {
	var deleteFunc func(cid int64, table string)
	deleteFunc = func(cid int64, table string) {
		query := fmt.Sprintf("DELETE FROM %s WHERE crawl_id = ? ORDER BY id DESC LIMIT 1000", table)
		_, err := ds.DB.Exec(query, cid)
		if err != nil {
			log.Printf("DeleteCrawlData: cid %d table %s %v\n", cid, table, err)
			return
		}

		query = fmt.Sprintf("SELECT count(*) FROM %s WHERE crawl_id = ?", table)
		row := ds.DB.QueryRow(query, cid)
		var c int
		if err := row.Scan(&c); err != nil {
			log.Printf("DeleteCrawlData count: pid %d table %s %v\n", cid, table, err)
		}

		if c > 0 {
			time.Sleep(1500 * time.Millisecond)
			deleteFunc(cid, table)
		}
	}

	deleteFunc(crawl.Id, "links")
	deleteFunc(crawl.Id, "external_links")
	deleteFunc(crawl.Id, "hreflangs")
	deleteFunc(crawl.Id, "issues")
	deleteFunc(crawl.Id, "images")
	deleteFunc(crawl.Id, "scripts")
	deleteFunc(crawl.Id, "styles")
	deleteFunc(crawl.Id, "iframes")
	deleteFunc(crawl.Id, "audios")
	deleteFunc(crawl.Id, "videos")
	deleteFunc(crawl.Id, "pagereports")
}

// DeleteProjectCrawls deletes all of the project's crawls and associated data.
func (ds *CrawlRepository) DeleteProjectCrawls(p *models.Project) {
	query := `
		SELECT
			id
		FROM crawls
		WHERE project_id = ?
	`

	rows, err := ds.DB.Query(query, p.Id)
	if err != nil {
		log.Printf("DeleteProjectCrawls Query: %v\n", err)
	}

	for rows.Next() {
		c := &models.Crawl{}
		if err := rows.Scan(&c.Id); err != nil {
			log.Printf("DeleteProjectCrawls: %v\n", err)
		}

		ds.DeleteCrawlData(c)
	}

	query = `DELETE FROM crawls WHERE project_id = ?`
	_, err = ds.DB.Exec(query, p.Id)
	if err != nil {
		log.Printf("deleting crawls for project %d: %v", p.Id, err)
		return
	}
}

// Deletes the unfinished crawls (issues_end is null) that have no page reports.
// Unfinished crawls with page reports are kept so they can be resumed.
// It cleans up the crawl data for each unfinished crawl before deleting it.
func (ds *CrawlRepository) DeleteUnfinishedCrawls() {
	query := `
		SELECT
			crawls.id
		FROM crawls
		WHERE crawls.issues_end IS NULL
			AND NOT EXISTS (SELECT 1 FROM pagereports WHERE pagereports.crawl_id = crawls.id)
	`
	count := 0

	rows, err := ds.DB.Query(query)
	if err != nil {
		log.Println(err)
		return
	}

	ids := []any{}
	placeholders := []string{}
	for rows.Next() {
		c := &models.Crawl{}
		err := rows.Scan(&c.Id)
		if err != nil {
			log.Printf("DeleteUnfinishedCrawls: %v\n", err)
			continue
		}

		count++
		ds.DeleteCrawlData(c)
		ids = append(ids, c.Id)
		placeholders = append(placeholders, "?")
	}

	if len(ids) == 0 {
		return
	}

	placeholdersStr := strings.Join(placeholders, ",")
	deleteQuery := fmt.Sprintf("DELETE FROM crawls WHERE id IN (%s)", placeholdersStr)
	_, err = ds.DB.Exec(deleteQuery, ids...)
	if err != nil {
		log.Printf("DeleteUnfinishedCrawls: %v", err)
	}

	log.Printf("Deleted %d unfinished crawls.", count)
}

// SaveIssuesCount stores the total number of issues as well as the total issues by priority for
// the crawl specified in the "crawlId" parameter.
func (ds *CrawlRepository) UpdateCrawl(crawl *models.Crawl) {
	query := `UPDATE
		crawls
		SET 
			end = ?,
			total_urls = ?,
			blocked_by_robotstxt = ?,
			noindex = ?,
			robotstxt_exists = ?,
			sitemap_exists = ?,
			sitemap_blocked = ?,
			links_internal_follow = ?,
			links_internal_nofollow = ?,
			links_external_follow = ?,
			links_external_nofollow = ?,
			links_sponsored = ?,
			links_ugc = ?,
			issues_end = ?,
			critical_issues = ?,
			alert_issues = ?,
			warning_issues = ?,
			total_issues = ?
		WHERE id = ?`

	_, err := ds.DB.Exec(
		query,
		crawl.End,
		crawl.TotalURLs,
		crawl.BlockedByRobotstxt,
		crawl.Noindex,
		crawl.RobotstxtExists,
		crawl.SitemapExists,
		crawl.SitemapIsBlocked,
		crawl.InternalFollowLinks,
		crawl.InternalNoFollowLinks,
		crawl.ExternalFollowLinks,
		crawl.ExternalNoFollowLinks,
		crawl.SponsoredLinks,
		crawl.UGCLinks,
		crawl.IssuesEnd,
		crawl.CriticalIssues,
		crawl.AlertIssues,
		crawl.WarningIssues,
		crawl.TotalIssues,
		crawl.Id,
	)
	if err != nil {
		log.Printf("SaveIssuesCount: %v\n", err)
	}
}

// TouchCrawl records that the crawl is still making progress.
func (ds *CrawlRepository) TouchCrawl(crawlId int64) {
	_, err := ds.DB.Exec("UPDATE crawls SET last_activity = CURRENT_TIMESTAMP WHERE id = ?", crawlId)
	if err != nil {
		log.Printf("TouchCrawl: %v\n", err)
	}
}

// MarkCrawlEnded stores the crawl's end time as soon as it stops requesting URLs,
// so the crawl shows as analysing issues while the multipage issues are created.
func (ds *CrawlRepository) MarkCrawlEnded(crawl *models.Crawl) {
	_, err := ds.DB.Exec("UPDATE crawls SET end = ?, last_activity = CURRENT_TIMESTAMP WHERE id = ?", crawl.End, crawl.Id)
	if err != nil {
		log.Printf("MarkCrawlEnded: %v\n", err)
	}
}

// GetLastFinishedCrawl returns the project's most recent finished crawl older than
// the crawl with the "beforeId" id. The returned crawl's Id is 0 if there's none.
func (ds *CrawlRepository) GetLastFinishedCrawl(p *models.Project, beforeId int64) models.Crawl {
	crawl := models.Crawl{ProjectId: p.Id}

	query := `
		SELECT id
		FROM crawls
		WHERE project_id = ? AND issues_end IS NOT NULL AND id < ?
		ORDER BY id DESC LIMIT 1`

	err := ds.DB.QueryRow(query, p.Id, beforeId).Scan(&crawl.Id)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("GetLastFinishedCrawl project id %d: %v\n", p.Id, err)
	}

	return crawl
}

// DeleteUnfinishedProjectCrawls deletes the project's unfinished crawls and their data,
// except for the crawl with the "exceptId" id.
func (ds *CrawlRepository) DeleteUnfinishedProjectCrawls(p *models.Project, exceptId int64) {
	query := `
		SELECT id
		FROM crawls
		WHERE project_id = ? AND issues_end IS NULL AND id != ?`

	rows, err := ds.DB.Query(query, p.Id, exceptId)
	if err != nil {
		log.Printf("DeleteUnfinishedProjectCrawls: %v\n", err)
		return
	}

	crawls := []*models.Crawl{}
	for rows.Next() {
		c := &models.Crawl{}
		if err := rows.Scan(&c.Id); err != nil {
			log.Printf("DeleteUnfinishedProjectCrawls: %v\n", err)
			continue
		}
		crawls = append(crawls, c)
	}
	rows.Close()

	for _, c := range crawls {
		ds.DeleteCrawlData(c)
		if _, err := ds.DB.Exec("DELETE FROM crawls WHERE id = ?", c.Id); err != nil {
			log.Printf("DeleteUnfinishedProjectCrawls: %v\n", err)
		}
	}
}

// CrawledURLs returns the URL of every page report stored for the crawl.
func (ds *CrawlRepository) CrawledURLs(crawlId int64) []string {
	urls := []string{}

	rows, err := ds.DB.Query("SELECT url FROM pagereports WHERE crawl_id = ?", crawlId)
	if err != nil {
		log.Printf("CrawledURLs: %v\n", err)
		return urls
	}
	defer rows.Close()

	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			log.Printf("CrawledURLs: %v\n", err)
			continue
		}
		urls = append(urls, u)
	}

	return urls
}

// FrontierURLs calls f with every URL the crawl's page reports point to: links, redirects,
// canonicals, hreflangs, iframes and resources. Depth is the depth of the page report the
// URL was found on plus one. Resource URLs are crawled whatever their domain, so they are
// passed with resource set to true. Nofollow links are skipped unless followNofollow is
// true and external links are only included when includeExternal is true.
// URLs may be repeated and may already be crawled; the crawler discards those.
func (ds *CrawlRepository) FrontierURLs(crawlId int64, followNofollow, includeExternal bool, f func(u string, depth int, resource bool)) {
	queries := []struct {
		query    string
		resource bool
	}{
		{"SELECT l.url, p.depth FROM links l JOIN pagereports p ON p.id = l.pagereport_id WHERE l.crawl_id = ? AND (l.nofollow = 0 OR ?)", false},
		{"SELECT p.redirect_url, p.depth FROM pagereports p WHERE p.crawl_id = ? AND p.redirect_url IS NOT NULL AND p.redirect_url != '' AND ? IS NOT NULL", false},
		{"SELECT p.canonical, p.depth FROM pagereports p WHERE p.crawl_id = ? AND p.canonical IS NOT NULL AND p.canonical != '' AND ? IS NOT NULL", false},
		{"SELECT h.to_url, p.depth FROM hreflangs h JOIN pagereports p ON p.id = h.pagereport_id WHERE h.crawl_id = ? AND ? IS NOT NULL", false},
		{"SELECT i.url, p.depth FROM iframes i JOIN pagereports p ON p.id = i.pagereport_id WHERE i.crawl_id = ? AND ? IS NOT NULL", false},
		{"SELECT i.url, p.depth FROM images i JOIN pagereports p ON p.id = i.pagereport_id WHERE i.crawl_id = ? AND ? IS NOT NULL", true},
		{"SELECT s.url, p.depth FROM scripts s JOIN pagereports p ON p.id = s.pagereport_id WHERE s.crawl_id = ? AND ? IS NOT NULL", true},
		{"SELECT s.url, p.depth FROM styles s JOIN pagereports p ON p.id = s.pagereport_id WHERE s.crawl_id = ? AND ? IS NOT NULL", true},
		{"SELECT a.url, p.depth FROM audios a JOIN pagereports p ON p.id = a.pagereport_id WHERE a.crawl_id = ? AND ? IS NOT NULL", true},
		{"SELECT v.url, p.depth FROM videos v JOIN pagereports p ON p.id = v.pagereport_id WHERE v.crawl_id = ? AND ? IS NOT NULL", true},
	}

	if includeExternal {
		queries = append(queries, struct {
			query    string
			resource bool
		}{"SELECT l.url, p.depth FROM external_links l JOIN pagereports p ON p.id = l.pagereport_id WHERE l.crawl_id = ? AND (l.nofollow = 0 OR ?)", false})
	}

	for _, q := range queries {
		rows, err := ds.DB.Query(q.query, crawlId, followNofollow)
		if err != nil {
			log.Printf("FrontierURLs: %v\n", err)
			continue
		}

		for rows.Next() {
			var u string
			var depth int
			if err := rows.Scan(&u, &depth); err != nil {
				log.Printf("FrontierURLs: %v\n", err)
				continue
			}
			f(u, depth+1, q.resource)
		}
		rows.Close()
	}
}

// RecountCrawl sets the crawl's URL and link totals from the data stored for it. A crawl
// keeps these totals in memory while it runs, so a resumed crawl recounts them first.
// URLs blocked by robots.txt are the page reports that were never requested
// (crawled = 0 and no status code).
func (ds *CrawlRepository) RecountCrawl(crawl *models.Crawl) {
	query := `
		SELECT
			(SELECT count(*) FROM pagereports WHERE crawl_id = ?),
			(SELECT count(*) FROM pagereports WHERE crawl_id = ? AND noindex = 1),
			(SELECT count(*) FROM pagereports WHERE crawl_id = ? AND crawled = 0 AND status_code = 0),
			(SELECT count(*) FROM links WHERE crawl_id = ? AND nofollow = 0),
			(SELECT count(*) FROM links WHERE crawl_id = ? AND nofollow = 1),
			(SELECT count(*) FROM external_links WHERE crawl_id = ? AND nofollow = 0),
			(SELECT count(*) FROM external_links WHERE crawl_id = ? AND nofollow = 1),
			(SELECT count(*) FROM external_links WHERE crawl_id = ? AND sponsored = 1),
			(SELECT count(*) FROM external_links WHERE crawl_id = ? AND ugc = 1)`

	id := crawl.Id
	err := ds.DB.QueryRow(query, id, id, id, id, id, id, id, id, id).Scan(
		&crawl.TotalURLs,
		&crawl.Noindex,
		&crawl.BlockedByRobotstxt,
		&crawl.InternalFollowLinks,
		&crawl.InternalNoFollowLinks,
		&crawl.ExternalFollowLinks,
		&crawl.ExternalNoFollowLinks,
		&crawl.SponsoredLinks,
		&crawl.UGCLinks,
	)
	if err != nil {
		log.Printf("RecountCrawl: %v\n", err)
	}
}
