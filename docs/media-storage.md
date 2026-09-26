# Media storage (Cloudflare R2, S3, MinIO)

Photos, videos, audio and documents can be kept in an S3-compatible bucket instead of only on the server's disk. The
bridge stores the location of every file in its database, so an agent or a person can always get to a media file, even
after WhatsApp has removed it from its own servers (it does that after a few weeks).

Works with Cloudflare R2, Amazon S3, MinIO, Backblaze B2, Wasabi and any service that speaks the S3 API.

## How a file travels

```
message arrives ─▶ store the message (never waits for media)
                └▶ download slot (MEDIA_DOWNLOAD_WORKERS) ─▶ streamed to store/media/<chat>/<id>.<ext>
                                                          └▶ row in message_media: pending_upload
upload worker (MEDIA_UPLOAD_WORKERS) ─▶ PutObject ─▶ row becomes "uploaded" (bucket + object key)
                                                  └▶ local copy deleted (unless kept)
```

- **The message is never delayed.** The message is stored first; media is handled by separate, bounded pools. A burst of
  thousands of messages means a longer queue, not thousands of simultaneous downloads or uploads.
- **The queue is the database.** Every file is a row in `message_media` with a status (`local`, `pending_upload`,
  `uploading`, `uploaded`, `failed`). A restart loses nothing: files that were `uploading` go back to the queue.
- **Files stream from disk.** Nothing is loaded whole into memory, so large videos fit the bridge's small container.
- **Failures retry by themselves** with a growing delay (30 s, 1 min, 2 min, ... up to 1 h, 8 attempts). A wrong
  key, a missing bucket or a missing permission does not use up attempts: the file waits and the error shows in
  *Settings > Media storage* until someone fixes the settings.
- **Nothing is lost while storage is off.** Files stay in `store/media/`; when you turn storage on, the ones already
  there are uploaded too.

## Setting it up

Open **Settings > Media storage** in the panel, fill the form, press *Test connection* (it writes and deletes a small
test object), then *Save*. Saving is refused if the test fails, so a bad password never becomes the active setting.

| Field | Cloudflare R2 | Amazon S3 | MinIO |
|-------|---------------|-----------|-------|
| Endpoint | `https://<account id>.r2.cloudflarestorage.com` | *(empty)* | `http://minio:9000` |
| Region | `auto` | e.g. `us-east-1` | `us-east-1` |
| Path-style | off | off | **on** |

Create an API token (R2: *Object Read & Write*, limited to the bucket) and paste its access key id and secret. The
secret is stored encrypted (AES-256-GCM) with a key derived from `API_KEY`, is never sent back to the browser, and is
never written to a log. If `API_KEY` changes, the stored secret can no longer be read and the panel asks for it again.

To manage it as code instead, set the variables below. They take priority and the panel form becomes read-only.

| Variable | Default | Description |
|----------|---------|-------------|
| `S3_BUCKET` | *(none)* | Setting it hands the configuration to the environment. |
| `S3_ENABLED` | `true` | Turn uploads on or off. |
| `S3_ENDPOINT` | *(none)* | Service URL; empty means Amazon S3. |
| `S3_REGION` | `auto` | Region. |
| `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY` | *(none)* | Credentials. |
| `S3_PATH_STYLE` | `false` | `host/bucket` addressing instead of `bucket.host`. |
| `S3_PREFIX` | *(none)* | Folder inside the bucket that holds everything. |
| `S3_KEEP_LOCAL` | `false` | Keep the local copy after the upload. Off frees the disk. |
| `S3_PUBLIC_BASE_URL` | *(none)* | Only for a public bucket or custom domain; otherwise links are signed and expire. |
| `MEDIA_DOWNLOAD_WORKERS` | `4` | Simultaneous downloads from WhatsApp. |
| `MEDIA_UPLOAD_WORKERS` | `4` | Simultaneous uploads to the bucket. |

## Folders in the bucket

```
{prefix}/{department}-{id}/{employee}-{id}/{number}/{chat name}__{chat id}/{yyyy-mm}/{message id}.{ext}

whatsapp/comercial-1/ana-souza-4/5511999990000/joao-cliente__5511988887777/2026-09/3EB028A580CF7CC9AA.jpg
```

- Department and employee are whoever held the number **when the message was exchanged**, the same attribution the
  audit feed uses. A number with nobody assigned goes under `sem-setor/sem-responsavel`.
- Names are lowercased ASCII (`João` becomes `joao`). The numeric ids keep two people with the same name apart.
- Groups end in `-g`, linked devices in `-lid`. Documents keep their original name after the message id.
- **The folder of a chat is decided once** (table `media_folders`) and reused. If someone changes department, or a
  contact is renamed, media already stored stays where it is and the conversation is not split; the database is the
  source of truth for where each file is.
- The bucket's own browser (for example the Cloudflare dashboard) shows these prefixes as folders.

## Reading a file back

- `POST /api/download` (used by the MCP `download_media` tool) serves the local copy when there is one, otherwise it
  reads the file from the bucket and returns a local path, as before. The WhatsApp CDN is only asked when neither has it.
- `GET /api/media/url?chat_jid=...&message_id=...[&instance_jid=...][&ttl=seconds]` returns a link. For a private bucket
  the link is signed and expires (default 15 minutes, at most 24 hours).

**Keep the bucket private.** Access then goes through the bridge, which has already authenticated the caller and applies
the access rules of [governance](governance.md). Anyone with a link, or with credentials to the bucket, bypasses those
rules and the access log. Use `S3_PUBLIC_BASE_URL` only for media that is fine to publish.

## Operating it

- *Settings > Media storage* shows the queue: uploaded, waiting, uploading, local only, failed. *Retry failed* puts
  failed files back in the queue.
- Downloads are recorded too: a file that WhatsApp no longer serves shows as `failed` with the reason.
- Nothing deletes objects from the bucket. Retention (`RETENTION_DAYS`) applies to the database; use a lifecycle rule on
  the bucket to expire objects.
- The bridge container is limited to 256 MB. Uploads stream from disk with one part in flight each, so the default of 4
  workers fits; raise `MEDIA_UPLOAD_WORKERS` together with the container's memory limit.

## Not covered yet

- Media of messages that arrived while the bridge was down, or through history sync, is not downloaded (its CDN link is
  usually expired by then).
- Stickers are not downloaded.
- The panel does not yet browse the bucket; use the bucket's own console, or `GET /api/media/url`.
