# feedzmore-media-worker

Generates video thumbnails for `feedzmore-service`. It has no database access: it talks only to
RabbitMQ and MinIO.

```
feedzmore-service ── video.thumbnail.requested ──▶ video.thumbnail.request.queue ──▶ worker
feedzmore-service ◀── video.thumbnail.generated ── video.thumbnail.result.queue  ◀── worker
```

1. `feedzmore-service` saves the post with a null thumbnail and publishes a request.
2. The worker downloads the video, extracts one frame with ffmpeg and uploads it to the requested key.
3. The worker publishes the result (with publisher confirm), then acks the request.
4. `feedzmore-service` stores the key on the post.

Exchange, queues and DLQs are declared by `feedzmore-service` (`RabbitMQConfig`). The worker never
declares topology.

## Messages (JSON, `content_type: application/json`)

Request, published to `post.exchange` with routing key `video.thumbnail.requested`:

```json
{"postId": "uuid", "mediaUrl": "user/POST_FEED/clip.mp4", "thumbnailObjectKey": "user/POST_FEED/clip_thumb.jpg"}
```

Result, published to `post.exchange` with routing key `video.thumbnail.generated`:

```json
{"postId": "uuid", "thumbnailObjectKey": "user/POST_FEED/clip_thumb.jpg"}
```

Keep these in sync with `VideoThumbnailRequestedEvent` / `VideoThumbnailGeneratedEvent` in `feedzmore-service`.

## Failure handling

- Transient errors are retried `WORKER_MAX_ATTEMPTS` times (1s, 2s, 4s... capped at 5s).
- Malformed or invalid messages are not retried.
- After the last attempt the request is rejected without requeue and lands in `video.thumbnail.request.dlq`.
- Delivery is at-least-once. A redelivery rewrites the same object, so no deduplication is needed.
- Videos shorter than 1s fall back to the first frame.

## Configuration

See `.env.example`. Required: `RABBITMQ_USERNAME`, `RABBITMQ_PASSWORD`, `MINIO_ENDPOINT`,
`MINIO_BUCKET`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`.

Probes: `GET :8080/healthz` (liveness), `GET :8080/readyz` (ready once connected to RabbitMQ).

## Develop

```
go vet ./... && go test ./...
docker build --platform linux/amd64 -t feedzmore-media-worker .
```

The image is distroless plus a static ffmpeg binary (about 57 MB).
