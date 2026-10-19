# SEOnaut API

[![GitHub](https://img.shields.io/github/license/brideo/seonaut)](LICENSE) [![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/brideo/seonaut/test.yml)](https://github.com/brideo/seonaut/actions/workflows/test.yml)

An API-only fork of [SEOnaut](https://github.com/stjudewashere/seonaut), the open-source SEO auditing crawler.

The crawler, issue detection engine, database schema and migrations are unchanged from upstream. What's different:

- The HTML user interface, templates, translations and cookie sessions have been removed.
- A small JSON API, protected by a shared API key, creates users and projects and starts, stops or resumes crawls.
- An interrupted crawl is kept and can be resumed from where it stopped. The crawl limit and timeout are configurable.
- Crawl results are not exposed over HTTP. Consumers are expected to read them directly from the MySQL database.
- Every table name can carry a configurable prefix (default `seonaut_`) so SEOnaut can share a database with other services.

This fork is intended to run as an internal service on a private Docker network, driven by another service that holds the API key and has access to the same database.

## Quick start

```sh
git clone https://github.com/brideo/seonaut.git
cd seonaut
export SEONAUT_API_KEY="$(openssl rand -hex 32)"
docker compose up --build -d
curl localhost:9000/health
```

The compose file starts MySQL 8.4 and the API on port 9000. The database is also published on host port 3302 so tools like TablePlus can connect with user `seonaut`, password `seonaut`, database `seonaut`. Migrations run automatically on start-up.

The server refuses to start without an API key.

## Authentication

Every request under `/api/` must carry the key, either as:

```
X-API-Key: <key>
```

or

```
Authorization: Bearer <key>
```

Requests without a valid key receive `401` with the standard error envelope. `GET /health` is public.

The key is a single shared secret. Any caller holding it can act on any user or project, so keep the service off public networks.

## Endpoints

| Method | Path | Description | Success |
|---|---|---|---|
| `GET` | `/health` | Liveness check, pings the database | `200` |
| `POST` | `/api/users` | Create a user | `201` |
| `POST` | `/api/projects` | Create a project owned by a user | `201` |
| `POST` | `/api/projects/{id}/crawl` | Start crawling a project | `202` |
| `POST` | `/api/projects/{id}/crawl/stop` | Stop the running crawl | `202` |
| `POST` | `/api/projects/{id}/crawl/resume` | Resume the project's unfinished crawl | `202` |

Request bodies are JSON. Unknown fields are rejected with `400`. Bodies larger than 1 MB are rejected.

### Create a user

```sh
curl -X POST localhost:9000/api/users \
  -H "X-API-Key: $SEONAUT_API_KEY" -H 'Content-Type: application/json' \
  -d '{"email":"crawler@example.com","password":"change-me"}'
```

```json
{"id":1,"email":"crawler@example.com"}
```

### Create a project

```sh
curl -X POST localhost:9000/api/projects \
  -H "X-API-Key: $SEONAUT_API_KEY" -H 'Content-Type: application/json' \
  -d '{"user_id":1,"url":"https://example.com","crawl_sitemap":true}'
```

| Field | Type | Default | Notes |
|---|---|---|---|
| `user_id` | int | required | Owner of the project |
| `url` | string | required | `http` or `https` only |
| `ignore_robotstxt` | bool | `false` | |
| `follow_nofollow` | bool | `false` | |
| `include_noindex` | bool | `false` | |
| `crawl_sitemap` | bool | `false` | |
| `allow_subdomains` | bool | `false` | |
| `check_external_links` | bool | `false` | |
| `basic_auth` | bool | `false` | When true, the crawl request must supply credentials |
| `archive` | bool | `false` | Save a WACZ archive of the crawl under `archive/<project id>/` |
| `user_agent` | string | `[crawler] agent` from config | |

```json
{
  "id": 1,
  "user_id": 1,
  "url": "https://example.com",
  "host": "example.com",
  "ignore_robotstxt": false,
  "follow_nofollow": false,
  "include_noindex": false,
  "crawl_sitemap": true,
  "allow_subdomains": false,
  "check_external_links": false,
  "basic_auth": false,
  "archive": false,
  "user_agent": "Mozilla/5.0 (compatible; SEOnautBot/1.0; +https://seonaut.org/bot)",
  "created": "2026-10-13T05:34:41Z"
}
```

### Start a crawl

```sh
curl -X POST localhost:9000/api/projects/1/crawl -H "X-API-Key: $SEONAUT_API_KEY"
```

The body is optional. For projects created with `basic_auth: true`, send the site credentials:

```json
{"auth_user":"name","auth_pass":"secret"}
```

```json
{"id":1,"project_id":1,"url":"https://example.com","start":"2026-10-13T05:34:41Z","crawling":true}
```

The crawl runs in the background. Only one crawl per project can run at a time; a second request returns `409`. Starting a new crawl deletes the data from the project's previous crawl once the new one finishes.

### Stop a crawl

```sh
curl -X POST localhost:9000/api/projects/1/crawl/stop -H "X-API-Key: $SEONAUT_API_KEY"
```

```json
{"stopping":true}
```

Returns `404` if the project has no running crawl.

### Resume a crawl

```sh
curl -X POST localhost:9000/api/projects/1/crawl/resume -H "X-API-Key: $SEONAUT_API_KEY"
```

Takes the same optional body as starting a crawl and returns the same response, with the id of the crawl being resumed.

The project's last crawl continues under the same id. The URLs it already crawled are skipped, and every URL they link to that hasn't been crawled yet is queued, along with the sitemap. URLs crawled before the resume count towards the crawl limit, and the resumed crawl gets a full timeout of its own. A crawler that is still registered for the project but has stopped responding is replaced.

Returns `409 nothing_to_resume` if the last crawl has finished, and `409 crawl_in_progress` if it has finished crawling and is creating its issues.

### Errors

All errors share one envelope:

```json
{"error":{"code":"crawl_in_progress","message":"project is already being crawled"}}
```

| Status | Code | When |
|---|---|---|
| `400` | `bad_request` | Malformed JSON, unknown field, invalid id |
| `400` | `validation_error` | Invalid email or password, unsupported URL scheme, empty user agent, unknown `user_id`, missing basic auth credentials |
| `401` | `unauthorized` | Missing or wrong API key |
| `404` | `not_found` | Unknown route or project, or no active crawl to stop |
| `405` | `method_not_allowed` | |
| `409` | `user_exists` | Email already registered |
| `409` | `crawl_in_progress` | Project already being crawled, or creating its issues when resumed |
| `409` | `nothing_to_resume` | The project's last crawl has finished |
| `409` | `project_deleting` | Project is being deleted |
| `500` | `internal_error` | Anything else. Details are logged, not returned |

## Reading results

Results live in MySQL. With the default prefix the main tables are:

| Table | Contents |
|---|---|
| `seonaut_crawls` | One row per crawl: `start`, `end`, `issues_end`, `last_activity`, `total_urls`, `total_issues`, critical/alert/warning counts. `end` is `NULL` while crawling |
| `seonaut_pagereports` | One row per crawled URL: status code, title, description, canonical, depth, etc. |
| `seonaut_issues` | Issues found, joined to `seonaut_pagereports` and `seonaut_issue_types` |
| `seonaut_issue_types` | Issue type names and priorities |
| `seonaut_links`, `seonaut_external_links`, `seonaut_images`, ... | Per-page resources |

`end` is set when the crawl stops requesting URLs and `issues_end` when its issues have been created, which is when the crawl is finished. `total_urls` and the link totals are only written when it finishes.

While a crawl runs, `last_activity` is updated every 15 seconds. A crawl with a `NULL` `end` whose `last_activity` has stopped moving has stalled or was interrupted by a restart, and can be resumed. On start-up the service deletes the unfinished crawls with no page reports and keeps the others for resuming. Starting a new crawl deletes the project's unfinished crawls once it finishes.

## Configuration

Settings come from the `config` TOML file and can be overridden with `SEONAUT_<SECTION>_<KEY>` environment variables.

| Config key | Environment variable | Default | Description |
|---|---|---|---|
| `server.host` | `SEONAUT_SERVER_HOST` | `0.0.0.0` | Bind address |
| `server.port` | `SEONAUT_SERVER_PORT` | `9000` | Port |
| `database.server` | `SEONAUT_DATABASE_SERVER` | `db` | MySQL host |
| `database.port` | `SEONAUT_DATABASE_PORT` | `3306` | |
| `database.user` | `SEONAUT_DATABASE_USER` | `seonaut` | |
| `database.password` | `SEONAUT_DATABASE_PASSWORD` | `seonaut` | |
| `database.database` | `SEONAUT_DATABASE_DATABASE` | `seonaut` | |
| `database.table_prefix` | `SEONAUT_DATABASE_TABLE_PREFIX` | `seonaut_` | Prefix for every table and foreign key name. Set to an empty string for upstream-compatible names |
| `crawler.agent` | `SEONAUT_CRAWLER_AGENT` | SEOnautBot UA | Default crawler user agent |
| `crawler.limit` | `SEONAUT_CRAWLER_LIMIT` | `20000` | Maximum URLs per crawl, including any crawled before it was resumed |
| `crawler.timeout_hours` | `SEONAUT_CRAWLER_TIMEOUT_HOURS` | `2` | Hours a crawl, or a resumed part of one, may run before it is stopped |
| `api.key` | `SEONAUT_API_KEY` | none, required | Shared API secret |

The table prefix is applied at runtime: migrations and queries are rewritten as they run, so the SQL in the repository keeps upstream's table names. Pick the prefix once before the first start; changing it later leaves the old tables behind.

## Development

Requires Go 1.25 and Docker.

```sh
make test     # go test -race ./internal/...
make vet
make docker   # compose with the db port published on 3306
```

For local runs outside Docker, copy `config` to `config.local`, point it at your MySQL instance, and run `make run` from the repository root (the `migrations` and `archive` paths are relative to the working directory).

## Syncing with upstream

```sh
git remote add upstream https://github.com/stjudewashere/seonaut.git
git fetch upstream
git merge upstream/main
```

Changes to `internal/crawler`, `internal/issues`, `internal/repository` and `migrations` merge cleanly. Upstream changes to the removed UI (`web/`, `internal/routes`, `translations/`) should be dropped. If an upstream migration creates a new table, add its name to `tableNames` in `internal/repository/conn.go`.

## License

SEOnaut is open-source under the MIT license. See [License File](LICENSE) for more information.
