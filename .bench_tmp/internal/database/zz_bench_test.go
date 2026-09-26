package database

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSerialBulkThroughput(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(dir)
	s, err := NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.StoreChat("c@g.us", "c", time.Now())
	n := 200
	start := time.Now()
	for i := 0; i < n; i++ {
		if err := s.StoreMessageBulk(fmt.Sprintf("id%d", i), "c@g.us", "s", "s", "hello", time.Now(), false, "", "", "", "", nil, nil, nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	el := time.Since(start)
	t.Logf("bulk serial: %d msgs in %v => %.0f msg/s", n, el, float64(n)/el.Seconds())

	// legacy: direct exec
	start = time.Now()
	for i := 0; i < n; i++ {
		_, err := s.db.Exec(`INSERT OR REPLACE INTO messages (id, chat_jid, sender, content, timestamp) VALUES (?,?,?,?,?)`, fmt.Sprintf("d%d", i), "c@g.us", "s", "hello", time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	el = time.Since(start)
	t.Logf("direct exec: %d msgs in %v => %.0f msg/s", n, el, float64(n)/el.Seconds())

	// PK collision: same group message seen by two instances
	_ = s.StoreMessageWithInstance("same", "g@g.us", "x", "x", "hi", time.Now(), false, "", "", "", "", nil, nil, nil, 0, "joao@s.whatsapp.net", false)
	_ = s.StoreMessageWithInstance("same", "g@g.us", "x", "x", "hi", time.Now(), false, "", "", "", "", nil, nil, nil, 0, "maria@s.whatsapp.net", false)
	var cnt int
	var inst string
	_ = s.db.QueryRow(`SELECT COUNT(*), MAX(instance_jid) FROM messages WHERE id='same'`).Scan(&cnt, &inst)
	t.Logf("same msg via 2 instances => rows=%d, instance_jid=%s", cnt, inst)
}
