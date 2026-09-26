package database

import (
	"database/sql"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Folder names used when the message has no department, employee or number attached.
const (
	noDepartment = "sem-setor"
	noEmployee   = "sem-responsavel"
	noNumber     = "sem-numero"
)

// maxSlugLen keeps folder names readable in a bucket browser.
const maxSlugLen = 40

// Slug turns a display name into a lowercase ASCII folder name ("João  da Silva" -> "joao-da-silva").
// It returns "" when nothing usable is left.
func Slug(name string) string {
	var b strings.Builder
	dash := true // suppresses a leading dash
	for _, r := range norm.NFD.String(name) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue // accent marks split off by NFD
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(unicode.ToLower(r))
			dash = false
		default:
			if !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > maxSlugLen {
		out = strings.TrimRight(out[:maxSlugLen], "-")
	}
	return out
}

// namedFolder joins a slug with the row id: the id keeps two people named alike apart and keeps
// the folder identifiable after a rename.
func namedFolder(name string, id int64, fallback string) string {
	if id == 0 {
		return fallback
	}
	if s := Slug(name); s != "" {
		return fmt.Sprintf("%s-%d", s, id)
	}
	return fmt.Sprintf("%d", id)
}

// chatFolder builds the folder of a conversation: the chat name for people to recognise it, then
// the JID for it to be unique ("joao-silva__5511988887777", "familia__1203630@g.us" -> "-g").
func chatFolder(chatJID, chatName string) string {
	user, server, _ := strings.Cut(chatJID, "@")
	id := Slug(user)
	if id == "" {
		id = "chat"
	}
	switch server {
	case "g.us":
		id += "-g"
	case "lid":
		id += "-lid"
	case "broadcast":
		id += "-bc"
	case "newsletter":
		id += "-nl"
	}
	if s := Slug(chatName); s != "" && s != id {
		return s + "__" + id
	}
	return id
}

// MediaFolderPrefix returns the bucket folder for a chat's media, ending in "/":
//
//	{department}/{employee}/{number}/{chat}/
//
// Department and employee are whoever held the number when the message was exchanged
// (instance_assignments), the same attribution the audit feed uses. The first answer is stored
// and reused, so a later rename or reassignment does not split a conversation across folders.
func (store *MessageStore) MediaFolderPrefix(instanceJID, chatJID string, messageTime time.Time) (string, error) {
	var prefix string
	err := store.db.QueryRow(
		`SELECT prefix FROM media_folders WHERE instance_jid = ? AND chat_jid = ?`, instanceJID, chatJID).Scan(&prefix)
	if err == nil {
		return prefix, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("read media folder: %w", err)
	}

	var (
		deptID, empID     int64
		deptName, empName string
	)
	if instanceJID != "" {
		err = store.db.QueryRow(
			`SELECT COALESCE(d.id, 0), COALESCE(d.name, ''), COALESCE(e.id, 0), COALESCE(e.name, '')
			 FROM instances i
			 LEFT JOIN instance_assignments a ON a.instance_id = i.id
			     AND datetime(?) >= datetime(a.valid_from)
			     AND (a.valid_to IS NULL OR datetime(?) < datetime(a.valid_to))
			 LEFT JOIN employees e ON e.id = a.employee_id
			 LEFT JOIN departments d ON d.id = e.department_id
			 WHERE i.phone_jid = ?
			 ORDER BY a.valid_from DESC LIMIT 1`,
			messageTime, messageTime, instanceJID).Scan(&deptID, &deptName, &empID, &empName)
		if err != nil && err != sql.ErrNoRows {
			return "", fmt.Errorf("resolve media owner: %w", err)
		}
	}

	var chatName string
	if err := store.db.QueryRow(`SELECT COALESCE(name, '') FROM chats WHERE jid = ?`, chatJID).Scan(&chatName); err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("resolve chat name: %w", err)
	}

	number := noNumber
	if user, _, _ := strings.Cut(instanceJID, "@"); Slug(user) != "" {
		number = Slug(user)
	}

	prefix = strings.Join([]string{
		namedFolder(deptName, deptID, noDepartment),
		namedFolder(empName, empID, noEmployee),
		number,
		chatFolder(chatJID, chatName),
	}, "/") + "/"

	// INSERT OR IGNORE then read back: two uploads of the same chat racing each other must end
	// up in the same folder.
	err = store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT OR IGNORE INTO media_folders (instance_jid, chat_jid, prefix) VALUES (?, ?, ?)`,
			instanceJID, chatJID, prefix)
		return err
	}, true, true)
	if err != nil {
		return "", fmt.Errorf("save media folder: %w", err)
	}
	if err := store.db.QueryRow(
		`SELECT prefix FROM media_folders WHERE instance_jid = ? AND chat_jid = ?`, instanceJID, chatJID).Scan(&prefix); err != nil {
		return "", fmt.Errorf("read media folder: %w", err)
	}
	return prefix, nil
}

// defaultExt is the extension used when the message carries no usable file name.
var defaultExt = map[string]string{
	"image":    ".jpg",
	"video":    ".mp4",
	"audio":    ".ogg",
	"document": ".bin",
}

// contentTypes covers the extensions the standard library does not know.
var contentTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
	".mp4":  "video/mp4",
	".3gp":  "video/3gpp",
	".mov":  "video/quicktime",
	".ogg":  "audio/ogg",
	".opus": "audio/ogg",
	".mp3":  "audio/mpeg",
	".m4a":  "audio/mp4",
	".pdf":  "application/pdf",
}

// MediaContentType guesses the MIME type from the file name, falling back to the media kind.
func MediaContentType(mediaType, filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	switch mediaType {
	case "image":
		return "image/jpeg"
	case "video":
		return "video/mp4"
	case "audio":
		return "audio/ogg"
	}
	return "application/octet-stream"
}

// safeFileName keeps letters, digits, dot, dash and underscore of an original file name.
func safeFileName(name string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(filepath.Base(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_'):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "._")
}

// MediaObjectKey builds the key of one file under its chat folder: {prefix}{yyyy-mm}/{message id}{ext}.
// Documents keep their original name after the message id, so the file is recognisable when
// browsed. The message id makes the key unique and stable, so uploading twice is harmless.
func MediaObjectKey(prefix string, rec *MediaRecord) string {
	month := rec.MessageTime.UTC().Format("2006-01")
	if rec.MessageTime.IsZero() {
		month = "sem-data"
	}
	id := safeFileName(rec.MessageID)
	if id == "" {
		id = "media"
	}
	ext := strings.ToLower(filepath.Ext(rec.Filename))
	if ext == "" {
		ext = defaultExt[rec.MediaType]
	}
	name := id + ext
	if rec.MediaType == "document" {
		if orig := safeFileName(rec.Filename); orig != "" {
			name = id + "_" + orig
		}
	}
	return prefix + month + "/" + name
}
