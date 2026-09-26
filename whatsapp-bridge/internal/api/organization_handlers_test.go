package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsapp-bridge/internal/config"
	"whatsapp-bridge/internal/database"
	"whatsapp-bridge/internal/whatsapp"
)

const (
	numA = "5511900000001"
	numB = "5511900000002"
)

// newTestServer runs in a temporary directory with a real store, a session store holding the given
// paired numbers, and an instance manager: the same wiring main.go builds.
func newTestServer(t *testing.T, paired ...string) *Server {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("API_KEY", "test-key")

	store, err := database.NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if len(paired) > 0 {
		container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", waLog.Noop)
		if err != nil {
			t.Fatal(err)
		}
		for _, user := range paired {
			dev := container.NewDevice()
			jid := types.JID{User: user, Device: 7, Server: types.DefaultUserServer}
			dev.ID = &jid
			dev.Account = &waAdv.ADVSignedDeviceIdentity{Details: []byte{1}, AccountSignature: make([]byte, 64), AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64)}
			if err := dev.Save(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		_ = container.Close()
	}

	mgr, err := whatsapp.NewInstanceManager(waLog.Noop, config.NewConfig(), store)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(mgr.GetDefaultClient(), store, nil, 0, "127.0.0.1", nil, "", "")
	s.SetInstanceManager(mgr)
	return s
}

// call runs a handler behind the same authentication the real routes use.
func call(t *testing.T, s *Server, h http.HandlerFunc, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set("X-API-Key", "test-key")
	req.Header.Set("X-Actor", "test-suite")
	rec := httptest.NewRecorder()
	s.SecureMiddleware(h)(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %q", rec.Body.String())
	}
	return out
}

func TestOutboundRoutesNeverGuessTheSender(t *testing.T) {
	s := newTestServer(t, numA, numB)
	send := s.outbound(s.handleSendMessage)
	body := map[string]any{"recipient": "5511977770000@s.whatsapp.net", "message": "oi"}

	if rec := call(t, s, send, http.MethodPost, "/api/send", body); rec.Code != http.StatusBadRequest {
		t.Errorf("with two numbers and no instance the request must be refused, got %d: %s", rec.Code, rec.Body)
	}
	if rec := call(t, s, send, http.MethodPost, "/api/send?instance=5599000000000", body); rec.Code != http.StatusNotFound {
		t.Errorf("unknown instance should be 404, got %d", rec.Code)
	}

	// Sending is off for a number until it is enabled.
	instB, _ := s.messageStore.GetInstanceByPhone(numB + "@s.whatsapp.net")
	no := false
	if _, err := s.messageStore.UpdateInstance(instB.ID, database.InstanceUpdate{AllowSend: &no}); err != nil {
		t.Fatal(err)
	}
	if rec := call(t, s, send, http.MethodPost, "/api/send?instance="+numB, body); rec.Code != http.StatusForbidden {
		t.Errorf("a number with sending disabled must be refused, got %d", rec.Code)
	}
	// An enabled number gets past the gate (the client is offline, so the send itself fails).
	if rec := call(t, s, send, http.MethodPost, "/api/send?instance="+numA, body); rec.Code == http.StatusBadRequest || rec.Code == http.StatusForbidden || rec.Code == http.StatusNotFound {
		t.Errorf("an enabled number should reach the handler, got %d: %s", rec.Code, rec.Body)
	}

	// Read-only routes may fall back to a default number.
	conn := s.scoped(s.handleConnectionStatus)
	if rec := call(t, s, conn, http.MethodGet, "/api/connection", nil); rec.Code != http.StatusOK {
		t.Errorf("read-only routes should not need an instance, got %d", rec.Code)
	}
}

func TestSingleNumberStillNeedsNoInstanceParameter(t *testing.T) {
	s := newTestServer(t, numA)
	send := s.outbound(s.handleSendMessage)
	rec := call(t, s, send, http.MethodPost, "/api/send", map[string]any{"recipient": "5511977770000@s.whatsapp.net", "message": "oi"})
	if rec.Code == http.StatusBadRequest || rec.Code == http.StatusForbidden || rec.Code == http.StatusNotFound {
		t.Errorf("single-number installs must keep working unchanged, got %d: %s", rec.Code, rec.Body)
	}
}

func TestInstanceEndpoints(t *testing.T) {
	s := newTestServer(t, numA)

	// Pairing requires the corporate-asset attestation.
	rec := call(t, s, s.handleInstances, http.MethodPost, "/api/instances", map[string]any{"alias": "Novo"})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "corporate_asset_confirmed") {
		t.Errorf("pairing without confirmation must be refused, got %d: %s", rec.Code, rec.Body)
	}

	list := decode(t, call(t, s, s.handleInstances, http.MethodGet, "/api/instances", nil))["instances"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected the loaded number, got %d", len(list))
	}
	inst := list[0].(map[string]any)
	id := int(inst["id"].(float64))
	if inst["phone_number"] != numA || inst["live"] != false {
		t.Errorf("unexpected instance payload: %v", inst)
	}

	// Link an employee, then unlink with null.
	emp := decode(t, call(t, s, s.handleEmployees, http.MethodPost, "/api/employees", map[string]any{"name": "Ana", "role": "Vendedora"}))["employee"].(map[string]any)
	empID := int(emp["id"].(float64))
	target := "/api/instances/" + strconv.Itoa(id)
	rec = call(t, s, s.handleInstanceByID, http.MethodPut, target, map[string]any{"employee_id": empID, "alias": "Vendas", "allow_send": false})
	got := decode(t, rec)["instance"].(map[string]any)
	if rec.Code != 200 || got["employee_name"] != "Ana" || got["alias"] != "Vendas" || got["allow_send"] != false {
		t.Errorf("update failed: %d %v", rec.Code, got)
	}
	got = decode(t, call(t, s, s.handleInstanceByID, http.MethodPut, target, map[string]any{"employee_id": nil}))["instance"].(map[string]any)
	if _, linked := got["employee_id"]; linked {
		t.Errorf("null should unlink the employee: %v", got)
	}
	if got["alias"] != "Vendas" {
		t.Errorf("fields not sent must be left alone: %v", got)
	}

	// Confirming records who and which wording.
	got = decode(t, call(t, s, s.handleInstanceByID, http.MethodPut, target, map[string]any{"corporate_asset_confirmed": true}))["instance"].(map[string]any)
	if got["corporate_asset_confirmed"] != true || got["corporate_confirmed_by"] != "api:test-suite" || got["corporate_terms_version"] != currentTermsVersion {
		t.Errorf("confirmation not recorded: %v", got)
	}

	if rec := call(t, s, s.handleInstanceByID, http.MethodGet, "/api/instances/99999", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown instance: %d", rec.Code)
	}
	if rec := call(t, s, s.handleInstanceByID, http.MethodPut, "/api/instances/99999", map[string]any{"alias": "x"}); rec.Code != http.StatusNotFound {
		t.Errorf("updating an unknown instance must be 404, got %d", rec.Code)
	}
	if rec := call(t, s, s.handleInstanceByID, http.MethodGet, "/api/instances/abc", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", rec.Code)
	}

	// Removing retires the number but keeps it (and its messages) attributable.
	if rec := call(t, s, s.handleInstanceByID, http.MethodDelete, target, nil); rec.Code != 200 {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	list = decode(t, call(t, s, s.handleInstances, http.MethodGet, "/api/instances", nil))["instances"].([]any)
	if len(list) != 0 {
		t.Errorf("removed instances are hidden by default, got %d", len(list))
	}
	list = decode(t, call(t, s, s.handleInstances, http.MethodGet, "/api/instances?include_removed=true", nil))["instances"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["status"] != "removed" {
		t.Errorf("removed instance should be listed on request: %v", list)
	}
}

func TestEmployeeUpdateIsPartial(t *testing.T) {
	s := newTestServer(t)
	dept := decode(t, call(t, s, s.handleDepartments, http.MethodPost, "/api/departments", map[string]any{"name": "Comercial"}))["department"].(map[string]any)
	emp := decode(t, call(t, s, s.handleEmployees, http.MethodPost, "/api/employees", map[string]any{"name": "Ana", "role": "Vendedora", "email": "ana@x.com", "department_id": dept["id"]}))["employee"].(map[string]any)
	target := "/api/employees/" + strconv.Itoa(int(emp["id"].(float64)))

	got := decode(t, call(t, s, s.handleEmployeeByID, http.MethodPut, target, map[string]any{"role": "Gerente"}))["employee"].(map[string]any)
	if got["role"] != "Gerente" || got["name"] != "Ana" || got["email"] != "ana@x.com" || got["active"] != true || got["department_name"] != "Comercial" {
		t.Errorf("a partial update must not reset other fields: %v", got)
	}
	got = decode(t, call(t, s, s.handleEmployeeByID, http.MethodPut, target, map[string]any{"active": false, "department_id": nil}))["employee"].(map[string]any)
	if got["active"] != false || got["department_name"] != nil {
		t.Errorf("explicit values should apply: %v", got)
	}
	if rec := call(t, s, s.handleEmployeeByID, http.MethodPut, "/api/employees/999", map[string]any{"role": "x"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown employee must be 404, got %d", rec.Code)
	}
}

func TestFeedIsFilteredAndLogged(t *testing.T) {
	s := newTestServer(t, numA)
	now := time.Now().UTC()
	jid := numA + "@s.whatsapp.net"
	_ = s.messageStore.StoreChatWithInstance("c@s.whatsapp.net", "Cliente", now, jid)
	_ = s.messageStore.StoreMessageWithInstance("M1", "c@s.whatsapp.net", "c", "c", "orçamento aprovado", now, false, "", "", "", "", nil, nil, nil, 0, jid, false)
	_, _ = s.messageStore.MarkMessageRemoteDeleted(jid, "M1", "c@s.whatsapp.net", "c", now)

	out := decode(t, call(t, s, s.handleMessageFeed, http.MethodGet, "/api/messages/feed?instance="+numA+"&deleted_only=true&q=or%C3%A7amento", nil))
	msgs := out["messages"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["is_deleted_remote"] != true {
		t.Fatalf("unexpected feed: %v", out)
	}

	for _, bad := range []string{"since=ontem", "until=x", "before=y"} {
		if rec := call(t, s, s.handleMessageFeed, http.MethodGet, "/api/messages/feed?"+bad, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s should be rejected, got %d", bad, rec.Code)
		}
	}

	entries, _ := s.messageStore.ListAccessLog(10)
	if len(entries) == 0 || entries[0].Action != "feed" || entries[0].Actor != "api:test-suite" {
		t.Errorf("reading the feed should be logged with the caller: %+v", entries)
	}

	// The MCP server records its own tool calls here.
	rec := call(t, s, s.handleAccessLog, http.MethodPost, "/api/access-log", map[string]any{"action": "tool:search", "params": "q=x"})
	if rec.Code != http.StatusCreated {
		t.Errorf("access log POST: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, s, s.handleAccessLog, http.MethodPost, "/api/access-log", map[string]any{"params": "no action"}); rec.Code != http.StatusBadRequest {
		t.Errorf("an entry without action must be refused, got %d", rec.Code)
	}
}

func TestPrivacyEndpoints(t *testing.T) {
	s := newTestServer(t, numA)
	now := time.Now().UTC()
	jid := numA + "@s.whatsapp.net"
	direct := "5511955550000@s.whatsapp.net"
	_ = s.messageStore.StoreChatWithInstance(direct, "Fulano", now, jid)
	_ = s.messageStore.StoreMessageWithInstance("P1", direct, "5511955550000", "Fulano", "meu CPF", now, false, "", "", "", "", nil, nil, nil, 0, jid, false)

	if rec := call(t, s, s.handlePrivacyAnonymize, http.MethodPost, "/api/privacy/anonymize", map[string]any{}); rec.Code != http.StatusBadRequest {
		t.Errorf("subject is required, got %d", rec.Code)
	}
	out := decode(t, call(t, s, s.handlePrivacyAnonymize, http.MethodPost, "/api/privacy/anonymize", map[string]any{"subject": "5511955550000"}))
	if out["messages"] != float64(1) || out["chats"] != float64(1) {
		t.Errorf("unexpected anonymize result: %v", out)
	}

	if rec := call(t, s, s.handlePrivacyPurge, http.MethodPost, "/api/privacy/purge", map[string]any{"days": 0}); rec.Code != http.StatusBadRequest {
		t.Errorf("days must be positive, got %d", rec.Code)
	}
	logOut := decode(t, call(t, s, s.handlePrivacyLog, http.MethodGet, "/api/privacy/log", nil))
	if entries := logOut["entries"].([]any); len(entries) != 1 || entries[0].(map[string]any)["actor"] != "api:test-suite" {
		t.Errorf("anonymization should be logged with the actor: %v", logOut)
	}
}

func TestHealthStaysUpWhileAnyNumberIsPaired(t *testing.T) {
	s := newTestServer(t, numA, numB)
	rec := call(t, s, s.handleHealth, http.MethodGet, "/api/health", nil)
	out := decode(t, rec)
	inst := out["instances"].(map[string]any)
	if inst["paired"] != float64(2) || inst["connected"] != float64(0) {
		t.Errorf("health should count numbers: %v", out)
	}
	if out["needs_pairing"] != false {
		t.Errorf("paired numbers do not need pairing: %v", out)
	}
}
