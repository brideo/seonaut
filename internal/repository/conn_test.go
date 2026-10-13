package repository_test

import (
	"testing"

	"github.com/stjudewashere/seonaut/internal/repository"
)

func TestRewrite(t *testing.T) {
	r := repository.NewRewriter("seonaut_")

	tests := []struct {
		in   string
		want string
	}{
		{"SELECT id FROM projects WHERE user_id = ?", "SELECT id FROM seonaut_projects WHERE user_id = ?"},
		{"INSERT INTO crawls (project_id) VALUES (?)", "INSERT INTO seonaut_crawls (project_id) VALUES (?)"},
		{"SELECT * FROM links l JOIN external_links e ON l.id = e.id", "SELECT * FROM seonaut_links l JOIN seonaut_external_links e ON l.id = e.id"},
		{"SELECT crawl_id, issue_type_id FROM issues", "SELECT crawl_id, issue_type_id FROM seonaut_issues"},
		{"CREATE TABLE IF NOT EXISTS `users` (id INT)", "CREATE TABLE IF NOT EXISTS `seonaut_users` (id INT)"},
		{"CONSTRAINT `projects_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)", "CONSTRAINT `seonaut_projects_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `seonaut_users` (`id`)"},
		{"CONSTRAINT audios_crawl FOREIGN KEY (crawl_id) REFERENCES crawls (id)", "CONSTRAINT `seonaut_audios_crawl` FOREIGN KEY (crawl_id) REFERENCES seonaut_crawls (id)"},
	}

	for _, tt := range tests {
		if got := r.Rewrite(tt.in); got != tt.want {
			t.Errorf("Rewrite(%q)\n got  %q\n want %q", tt.in, got, tt.want)
		}
	}
}

func TestRewriteEmptyPrefix(t *testing.T) {
	r := repository.NewRewriter("")
	q := "SELECT id FROM projects"
	if got := r.Rewrite(q); got != q {
		t.Errorf("empty prefix should not rewrite: got %q", got)
	}
}
