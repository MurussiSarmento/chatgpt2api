# Docker Deployment

## Deployment

```bash
git clone git@github.com:basketikun/chatgpt2api.git
cd chatgpt2api
cp config.example.yaml config.yaml
```

Edit `config.yaml` to set `api-key` (or override it via `CHATGPT2API_AUTH_KEY` in `.env`), then choose an orchestration file as needed:

| File | Image source | WARP proxy | Port |
| --- | --- | --- | --- |
| `docker-compose.yml` | Pull image | Without | 3000 |
| `docker-compose.warp.yml` | Pull image | With | 3000 |
| `docker-compose.local.yml` | Local build | Without | 8000 |
| `docker-compose.local.warp.yml` | Local build | With | 8000 |

```bash
docker compose up -d
```

Specify other files with `-f`, e.g. `docker compose -f docker-compose.warp.yml up -d`; add `--build` for a local build.

- Panel: `http://localhost:3000`
- API: `http://localhost:3000/v1`

You need to persist `config.yaml`, `.env`, and `data/`. The database defaults to SQLite (`data/chatgpt2api.db`) and can be switched to PostgreSQL / MySQL via `DATABASE_URL` in `.env`.

## WARP / FlareSolverr Stable Proxy

The orchestration files with `.warp` are used to mitigate Cloudflare blocks on the image route, and start the following together:

- `warp-proxy`: provides a WARP SOCKS5 exit.
- `privoxy`: converts the WARP SOCKS5 into an HTTP proxy.
- `flaresolverr`: refreshes the Cloudflare clearance.
- `app`: the ChatGPT2API main service.

To use it, set `proxy_runtime.enabled` to `true` in `config.yaml`; you can also save and test it on the admin settings page.

Only upstream OpenAI / ChatGPT requests go through the stable proxy; auxiliary routes such as account email and CPA are unaffected. Proxy priority: the account's own proxy > the stable proxy > the global `proxy` in `config.yaml`.

## Upgrade

```bash
git pull
docker compose pull
docker compose up -d
```

## Common Commands

```bash
docker compose ps
docker logs -f chatgpt2api
```
