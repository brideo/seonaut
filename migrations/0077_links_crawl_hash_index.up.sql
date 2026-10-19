ALTER TABLE links ADD INDEX IF NOT EXISTS `links_crawl_hash` (`crawl_id`, `url_hash`);
