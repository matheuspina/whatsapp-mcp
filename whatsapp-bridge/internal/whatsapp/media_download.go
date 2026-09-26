package whatsapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"

	"whatsapp-bridge/internal/database"
)

// downloadTimeout bounds one media download, retries included.
const downloadTimeout = 5 * time.Minute

// defaultMediaDownloads is how many downloads run at once.
const defaultMediaDownloads = 4

// MediaSink receives the outcome of every media download. The object storage manager implements
// it; without one, downloaded files simply stay in the local store folder.
type MediaSink interface {
	// Registered reports whether the message's media is already known (downloaded, queued or
	// uploaded), so a message delivered twice is not downloaded twice.
	Registered(instanceJID, chatJID, messageID string) bool
	// HandleMedia is called with a file that is on disk and complete.
	HandleMedia(rec database.MediaRecord)
	// HandleMediaFailure is called when the file could not be downloaded.
	HandleMediaFailure(rec database.MediaRecord, reason string)
}

var (
	mediaSink          MediaSink
	mediaDownloadSlots = make(chan struct{}, defaultMediaDownloads)
)

// SetMediaSink registers the receiver of downloaded media. Call it once at startup, before any
// client connects.
func SetMediaSink(sink MediaSink) { mediaSink = sink }

// SetMediaDownloadConcurrency limits how many media files download at the same time (default 4).
// Call it once at startup, before any client connects. Messages that arrive while every slot is
// busy wait for one: the message itself is stored immediately either way.
func SetMediaDownloadConcurrency(n int) {
	if n > 0 {
		mediaDownloadSlots = make(chan struct{}, n)
	}
}

// mediaJob is a media file to download: where it lives on the CDN and how to decrypt it.
type mediaJob struct {
	instanceJID   string
	chatJID       string
	messageID     string
	timestamp     time.Time
	mediaType     string
	filename      string
	url           string
	directPath    string
	mediaKey      []byte
	fileSHA256    []byte
	fileEncSHA256 []byte
	fileLength    uint64
}

var unsafePathChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// pathSegment makes untrusted text (a JID, a message id, a file name) safe as one path element.
func pathSegment(s string) string {
	s = strings.TrimLeft(unsafePathChars.ReplaceAllString(s, "_"), ".")
	if s == "" {
		return "_"
	}
	return s
}

// mediaExtension picks the local file extension: the original one when it is sane, else by kind.
func mediaExtension(mediaType, filename string) string {
	ext := strings.ToLower(filepath.Ext(pathSegment(filename)))
	if ext != "" && len(ext) <= 8 {
		return ext
	}
	switch mediaType {
	case "image":
		return ".jpg"
	case "video":
		return ".mp4"
	case "audio":
		return ".ogg"
	}
	return ".bin"
}

// localMediaPath is where a message's media lives on disk: store/media/<chat>/<message id><ext>.
// The message id, not the time of arrival, names the file, so the same message always maps to the
// same path.
func localMediaPath(chatJID, messageID, mediaType, filename string) string {
	return filepath.Join("store", "media", pathSegment(chatJID), pathSegment(messageID)+mediaExtension(mediaType, filename))
}

// whatsmeowMediaType maps the bridge's media kind to the key set used to decrypt it.
func whatsmeowMediaType(mediaType string) (whatsmeow.MediaType, error) {
	switch strings.ToLower(mediaType) {
	case "image":
		return whatsmeow.MediaImage, nil
	case "video":
		return whatsmeow.MediaVideo, nil
	case "audio":
		return whatsmeow.MediaAudio, nil
	case "document":
		return whatsmeow.MediaDocument, nil
	}
	return "", fmt.Errorf("unsupported media type %q", mediaType)
}

// autoDownloadMedia downloads a message's media to disk right after it arrives, while the CDN
// link is still valid, and hands the file to the media sink. It runs in its own goroutine and
// waits for a download slot, so a burst of messages never means a burst of simultaneous
// downloads. The file is streamed to disk, not held in memory. Errors are logged and recorded,
// never returned.
func (c *Client) autoDownloadMedia(job mediaJob) {
	mediaDownloadSlots <- struct{}{}
	defer func() { <-mediaDownloadSlots }()

	sink := mediaSink
	if sink != nil && sink.Registered(job.instanceJID, job.chatJID, job.messageID) {
		return
	}

	rec := database.MediaRecord{
		InstanceJID: job.instanceJID,
		ChatJID:     job.chatJID,
		MessageID:   job.messageID,
		MediaType:   job.mediaType,
		Filename:    job.filename,
		ContentType: database.MediaContentType(job.mediaType, job.filename),
		MessageTime: job.timestamp,
	}
	fail := func(reason string, err error) {
		c.logger.Warnf("media download: %s/%s: %s: %v", job.chatJID, job.messageID, reason, err)
		if sink != nil {
			sink.HandleMediaFailure(rec, fmt.Sprintf("%s: %v", reason, err))
		}
	}

	wmType, err := whatsmeowMediaType(job.mediaType)
	if err != nil {
		fail("classify", err)
		return
	}
	directPath := job.directPath
	if directPath == "" {
		directPath = extractDirectPath(job.url)
	}

	finalPath := localMediaPath(job.chatJID, job.messageID, job.mediaType, job.filename)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		fail("create folder", err)
		return
	}

	// A file left by an earlier run counts as downloaded.
	if info, err := os.Stat(finalPath); err == nil && info.Size() > 0 {
		rec.LocalPath, rec.Size = finalPath, info.Size()
		if sink != nil {
			sink.HandleMedia(rec)
		}
		return
	}

	// Download to a sibling .part file and rename when complete, so a crash or a failed
	// download never leaves a truncated file where a finished one is expected.
	partPath := finalPath + ".part"
	f, err := os.OpenFile(partPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		fail("create file", err)
		return
	}
	dl := &autoDownloadableMedia{
		url:           job.url,
		directPath:    directPath,
		mediaKey:      job.mediaKey,
		fileLength:    job.fileLength,
		fileSHA256:    job.fileSHA256,
		fileEncSHA256: job.fileEncSHA256,
		mediaType:     wmType,
	}
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	err = c.Client.DownloadToFile(ctx, dl, f)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(partPath)
		fail("download", err)
		return
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		_ = os.Remove(partPath)
		fail("finalize file", err)
		return
	}

	info, err := os.Stat(finalPath)
	if err != nil {
		fail("stat", err)
		return
	}
	rec.LocalPath, rec.Size = finalPath, info.Size()
	c.logger.Infof("media download: saved %s (%d bytes)", finalPath, rec.Size)
	if sink != nil {
		sink.HandleMedia(rec)
	}
}

// autoDownloadableMedia implements whatsmeow.DownloadableMessage (plus MediaTypeable) for media
// whose fields come from the database instead of a protobuf message.
type autoDownloadableMedia struct {
	url           string
	directPath    string
	mediaKey      []byte
	fileLength    uint64
	fileSHA256    []byte
	fileEncSHA256 []byte
	mediaType     whatsmeow.MediaType
}

func (d *autoDownloadableMedia) GetDirectPath() string             { return d.directPath }
func (d *autoDownloadableMedia) GetMediaKey() []byte               { return d.mediaKey }
func (d *autoDownloadableMedia) GetFileLength() uint64             { return d.fileLength }
func (d *autoDownloadableMedia) GetFileSHA256() []byte             { return d.fileSHA256 }
func (d *autoDownloadableMedia) GetFileEncSHA256() []byte          { return d.fileEncSHA256 }
func (d *autoDownloadableMedia) GetMediaType() whatsmeow.MediaType { return d.mediaType }
func (d *autoDownloadableMedia) GetUrl() string                    { return d.url }

// extractDirectPath strips the scheme and host from a WhatsApp CDN URL to get the path.
func extractDirectPath(rawURL string) string {
	if idx := strings.Index(rawURL, ".net/"); idx >= 0 {
		return rawURL[idx+4:]
	}
	if i := strings.Index(rawURL, "://"); i >= 0 {
		rest := rawURL[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			return rest[j:]
		}
	}
	return rawURL
}
