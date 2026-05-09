# crane-copy-fc

An Aliyun Function Compute (FC3) custom-container service that copies Docker images between registries using [`go-containerregistry`](https://github.com/google/go-containerregistry) (`crane.Copy`).

## API

### `POST /invoke`

Accepts a JSON payload with image pairs and copies them concurrently.

**Request body:**

```json
{
  "src_prefix": "docker.io/library",
  "dest_prefix": "registry.cn-hongkong.aliyuncs.com/myproj",
  "images": [
    ["nginx:1.25", "nginx:1.25"],
    ["redis:7.2", ""],
    ["busybox:1.36", "my-custom-busybox:1.36"]
  ],
  "max_concurrent": 3,
  "src_user": "",
  "src_pwd": "",
  "dest_user": "aliyun_acr_username",
  "dest_pwd": "aliyun_acr_password"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `src_prefix` | `string` | Source registry + namespace prefix (e.g. `docker.io/library`) |
| `dest_prefix` | `string` | Destination registry + namespace prefix (e.g. `registry.cn-hongkong.aliyuncs.com/myproj`) |
| `images` | `[][2]string` | Array of `[src_suffix, dest_suffix]` pairs. Full refs are built by joining prefix + suffix with `/`. If `dest_suffix` is empty (`""`), it defaults to the same value as `src_suffix`. |
| `max_concurrent` | `int` | Max parallel copies (default: 3) |
| `src_user` | `string` | Source registry username (optional) |
| `src_pwd` | `string` | Source registry password (optional) |
| `dest_user` | `string` | Destination registry username (optional) |
| `dest_pwd` | `string` | Destination registry password (optional) |

In the example above `["redis:7.2", ""]` copies `docker.io/library/redis:7.2` → `registry.cn-hongkong.aliyuncs.com/myproj/redis:7.2` because the empty dest defaults to the src name.

**Response:**

```json
{
  "results": [
    {"image": "nginx:1.25 -> nginx:1.25", "src": "docker.io/library/nginx:1.25", "dest": "registry.cn-hongkong.aliyuncs.com/myproj/nginx:1.25"},
    {"image": "busybox:1.36 -> my-custom-busybox:1.36", "src": "...", "dest": "...", "error": "..."}
  ]
}
```

Each image gets its own result entry. Errors on individual images do not abort the whole batch.

### Other endpoints

- `POST /initialize` — FC initialization hook, returns `{"message":"initialize ok"}`
- `GET /` — Health check, returns `{"message":"hello world"}`

## Local test

```bash
go run main.go

curl -X POST http://localhost:8080/invoke \
  -H 'Content-Type: application/json' \
  -d @example.event.json
```

## Deploy to Aliyun FC

Make sure you have [Serverless Devs](https://www.serverless-devs.com) installed and configured.

```bash
export REG_USER='your_registry_username'
export REG_PWD='your_registry_password'
s deploy -y
```

> **Note:** Update `s.yaml` `customContainerConfig.image` to your actual registry image URL before deploying.

## Post-deploy CLI

```bash
export REG_USER='...'
export REG_PWD='...'
s info    # show deployed function config
s logs    # tail recent invocation logs
```

## Project structure

```
.
├── main.go           # HTTP server
├── Dockerfile        # Multi-stage build (Alpine runtime)
├── Makefile          # podman build / run / push helpers
├── go.mod
├── go.sum
├── s.yaml            # Serverless Devs deployment config
├── example.event.json # Sample /invoke payload
├── build/
│   └── test.json     # Makefile curl test payload
├── .env.example      # Template for registry credentials
├── .dockerignore
└── .gitignore
```
