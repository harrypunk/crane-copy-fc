# crane-copy-fc

An Aliyun Function Compute (FC3) custom-container service that copies Docker images between registries using [`go-containerregistry`](https://github.com/google/go-containerregistry) (`crane.Copy`).

## API

### `POST /invoke`

Accepts a JSON payload with a list of image pairs and copies them concurrently.

**Request body:**

```json
{
  "images": [
    {"src": "docker.io/library/nginx:1.25", "dest": "registry.cn-hongkong.aliyuncs.com/myproj/nginx:1.25"}
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
| `images` | `[]object` | List of `{src, dest}` image references |
| `max_concurrent` | `int` | Max parallel copies (default: 3) |
| `src_user` | `string` | Source registry username (optional) |
| `src_pwd` | `string` | Source registry password (optional) |
| `dest_user` | `string` | Destination registry username (optional) |
| `dest_pwd` | `string` | Destination registry password (optional) |

**Response:**

```json
{
  "results": [
    {"src": "...", "dest": "..."},
    {"src": "...", "dest": "...", "error": "..."}
  ]
}
```

Each image gets its own result entry. Errors on individual images do not abort the whole batch.

### Other endpoints

- `POST /initialize` — FC initialization hook, returns `{"message":"initialize ok"}`
- `GET /` — Health check, returns `{"message":"hello world"}`

## Local test

```bash
cd code
go run main.go

curl -X POST http://localhost:8080/invoke \
  -H 'Content-Type: application/json' \
  -d @../example.event.json
```

## Deploy to Aliyun FC

Make sure you have [Serverless Devs](https://www.serverless-devs.com) installed and configured.

```bash
s deploy -y
```

> **Note:** Update `s.yaml` `customContainerConfig.image` to your actual ACR image URL before deploying.

## Project structure

```
.
├── code/
│   ├── main.go       # HTTP server
│   ├── Dockerfile    # Custom container build
│   ├── go.mod
│   └── go.sum
├── s.yaml            # Serverless Devs deployment config
└── example.event.json # Sample /invoke payload
```
