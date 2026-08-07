# Deployment

## CI Pipeline (`.gitlab-ci.yml`)

```
lint   → go vet + gofmt check
test   → go build + go test
build  → docker build (main branch + MR triggers)
push   → push to Harbor (main branch also tags latest)
```

## Image

- Base image: `alpine:latest` (recommend changing to `alpine:3.20` to pin the version)
- Final image only contains `server` binary + `ca-certificates` + `tzdata`
- Exposed port: `8081`
- Health check: `GET /health`

## Production Deployment Checklist

- [ ] Set `PROVIDER_ENCRYPTION_KEY` to a strong random value (do not commit to the repository)
- [ ] Explicitly configure `SERVER_CORS_ORIGINS` (do not use the default AllowAll)
- [ ] Run AutoMigrate as a separate Job
- [ ] Configure the `agents-admin` role in Casdoor
- [ ] Set the MinIO Bucket to public read or use CDN to accelerate Skill downloads
- [ ] Back up the database regularly (`cloud_sessions` / `cloud_messages` are user data)

## Single ECS Deployment Recommendation

See [architecture.md#single-ecs-deployment-recommendation](architecture.md#single-ecs-deployment-recommendation).
