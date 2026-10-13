# INSTALLATION GUIDE

This fork of SEOnaut exposes a JSON API only. See the [README](../README.md) for the endpoint reference. This document covers running it.

## Prerequisites

- **Docker** with compose, or
- **Go 1.25** and a **MySQL 8** server for running from source.

## Running with docker compose

The repository's `docker-compose.yml` starts MySQL and the API:

```sh
export SEONAUT_API_KEY="$(openssl rand -hex 32)"
docker compose up --build -d
```

`SEONAUT_API_KEY` is required; compose refuses to start without it. The API listens on port 9000 and MySQL is published on host port 3302.

To use the image from another compose project, copy the `app` service and point `SEONAUT_DATABASE_*` at your database:

```yml
services:
  seonaut:
    image: ghcr.io/brideo/seonaut:latest
    command: sh -c "/bin/wait && /app/seonaut"
    environment:
      - WAIT_HOSTS=db:3306
      - SEONAUT_API_KEY=${SEONAUT_API_KEY:?required}
      - SEONAUT_DATABASE_SERVER=db
      - SEONAUT_DATABASE_PORT=3306
      - SEONAUT_DATABASE_USER=seonaut
      - SEONAUT_DATABASE_PASSWORD=seonaut
      - SEONAUT_DATABASE_DATABASE=seonaut
      - SEONAUT_DATABASE_TABLE_PREFIX=seonaut_
    volumes:
      - seonaut-archive:/app/archive
```

Mount `/app/archive` if you use the `archive` project option and want the WACZ files to survive container restarts.

## Running from source

```sh
cp config config.local
# edit config.local: database settings and api key
make run
```

Or with an explicit config path:

```sh
go run -race cmd/server/main.go -c path/to/config
```

Run from the repository root: the `migrations` directory and the `archive` output directory are resolved relative to the working directory.

## Configuration file

```toml
[server]
host = "0.0.0.0"
port = 9000
url = "http://localhost:9000"

[database]
server = "db"
port = 3306
user = "seonaut"
password = "seonaut"
database = "seonaut"
table_prefix = "seonaut_"

[crawler]
agent = "Mozilla/5.0 (compatible; SEOnautBot/1.0; +https://seonaut.org/bot)"

[api]
key = ""
```

Every key can be overridden with an environment variable named `SEONAUT_<SECTION>_<KEY>`, for example `SEONAUT_DATABASE_TABLE_PREFIX` or `SEONAUT_API_KEY`.

- **[server]** `host` and `port` control where the API listens. `url` is kept for compatibility and is not used by the API.
- **[database]** standard MySQL connection settings. `table_prefix` is prepended to every table and foreign key name, including the migrations tracking table. Choose it before the first start.
- **[crawler]** `agent` is the default user agent for projects created without one.
- **[api]** `key` is the shared secret. The server exits on start-up if it is empty.

## Reverse proxy

The API has no websockets or static assets, so a plain HTTP reverse proxy is enough if you need TLS:

```nginx
server {
    listen 443 ssl;
    server_name seonaut.internal;

    location / {
        proxy_pass http://localhost:9000;
        proxy_set_header Host $host;
    }
}
```

Keep the service on a private network. The API key grants access to every user and project.
