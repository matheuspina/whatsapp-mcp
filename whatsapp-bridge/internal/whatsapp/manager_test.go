package whatsapp

import (
	"context"
	"strconv"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsapp-bridge/internal/config"
	"whatsapp-bridge/internal/database"
)

const (
	testJIDA = "5511900000001"
	testJIDB = "5511900000002"
)

// newManagerEnv runs in a temporary working directory with a real message store and a session
// store that already holds the given paired numbers.
func newManagerEnv(t *testing.T, paired ...string) (*InstanceManager, *database.MessageStore) {
	t.Helper()
	t.Chdir(t.TempDir())

	msgStore, err := database.NewMessageStore()
	if err != nil {
		t.Fatalf("NewMessageStore: %v", err)
	}
	t.Cleanup(func() { _ = msgStore.Close() })

	if len(paired) > 0 {
		container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", waLog.Noop)
		if err != nil {
			t.Fatalf("sqlstore.New: %v", err)
		}
		for _, user := range paired {
			dev := container.NewDevice()
			jid := types.JID{User: user, Device: 7, Server: types.DefaultUserServer}
			dev.ID = &jid
			dev.Account = testAccount()
			if err := dev.Save(context.Background()); err != nil {
				t.Fatalf("save device: %v", err)
			}
		}
		_ = container.Close()
	}

	mgr, err := NewInstanceManager(waLog.Noop, config.NewConfig(), msgStore)
	if err != nil {
		t.Fatalf("NewInstanceManager: %v", err)
	}
	return mgr, msgStore
}

// testAccount is the minimum account identity the session store needs to persist a device.
func testAccount() *waAdv.ADVSignedDeviceIdentity {
	return &waAdv.ADVSignedDeviceIdentity{
		Details:             []byte{1},
		AccountSignature:    make([]byte, 64),
		AccountSignatureKey: make([]byte, 32),
		DeviceSignature:     make([]byte, 64),
	}
}

func TestManagerStartsWithUnpairedDefaultDevice(t *testing.T) {
	mgr, _ := newManagerEnv(t)

	if def := mgr.GetDefaultClient(); def == nil || def.InstanceJID() != "" {
		t.Fatalf("expected an unpaired default client, got %+v", def)
	}
	if mgr.PairedCount() != 0 || len(mgr.ListClients()) != 1 {
		t.Errorf("unexpected pool: paired=%d total=%d", mgr.PairedCount(), len(mgr.ListClients()))
	}
	if _, ok := mgr.OnlyPairedClient(); ok {
		t.Error("no paired client should be reported")
	}
}

func TestManagerLoadsEveryPairedNumberAndRegistersInstances(t *testing.T) {
	mgr, store := newManagerEnv(t, testJIDA, testJIDB)

	if mgr.PairedCount() != 2 {
		t.Fatalf("expected 2 paired clients, got %d", mgr.PairedCount())
	}
	if _, ok := mgr.OnlyPairedClient(); ok {
		t.Error("with two numbers there is no unambiguous sender")
	}

	instances, err := store.ListInstances(false)
	if err != nil || len(instances) != 2 {
		t.Fatalf("expected 2 instance rows, got %d (err=%v)", len(instances), err)
	}
	for _, inst := range instances {
		if inst.Status != database.InstanceStatusDisconnected || !inst.AllowSend {
			t.Errorf("loaded instance should start disconnected with the default send permission: %+v", inst)
		}
	}

	// A number is resolvable by JID, by device-qualified JID, by bare phone number and by id.
	inst, _ := store.GetInstanceByPhone(testJIDA + "@s.whatsapp.net")
	for _, ref := range []string{testJIDA + "@s.whatsapp.net", testJIDA + ":7@s.whatsapp.net", testJIDA, itoa(inst.ID)} {
		c, err := mgr.ResolveClient(ref)
		if err != nil || c.InstanceJID() != testJIDA+"@s.whatsapp.net" {
			t.Errorf("ResolveClient(%q) = %v, %v", ref, c.InstanceJID(), err)
		}
	}
	if _, err := mgr.ResolveClient("5599000000000"); err != ErrInstanceNotFound {
		t.Errorf("unknown number should be ErrInstanceNotFound, got %v", err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestManagerAttributesLegacyMessagesToTheOnlyNumber(t *testing.T) {
	t.Chdir(t.TempDir())
	msgStore, err := database.NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	defer msgStore.Close()
	_ = msgStore.StoreChat("c@s.whatsapp.net", "C", time.Now())
	_ = msgStore.StoreMessage("OLD", "c@s.whatsapp.net", "c", "c", "antigo", time.Now(), false, "", "", "", "", nil, nil, nil, 0)
	msgStore.Close()

	// Same directory, now with one paired number.
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	dev := container.NewDevice()
	jid := types.JID{User: testJIDA, Device: 1, Server: types.DefaultUserServer}
	dev.ID = &jid
	dev.Account = testAccount()
	if err := dev.Save(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = container.Close()

	msgStore, err = database.NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	defer msgStore.Close()
	if _, err := NewInstanceManager(waLog.Noop, config.NewConfig(), msgStore); err != nil {
		t.Fatal(err)
	}
	if n, _ := msgStore.CountUnattributedMessages(); n != 0 {
		t.Errorf("the earlier message should now belong to the only paired number, %d remain", n)
	}
}

func TestPairSuccessBindsThePendingInstance(t *testing.T) {
	mgr, store := newManagerEnv(t, testJIDA)
	hooked := make(chan string, 1)
	mgr.AddClientHook(func(ctx context.Context, c *Client) { hooked <- c.InstanceJID() })
	<-hooked // the already-paired number

	pending, err := store.CreatePendingInstance("Novo", nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	dev := mgr.container.NewDevice()
	client, err := NewClientForDeviceWithOptions(dev, waLog.Noop, mgr.cfg, ClientOptions{NoAntibanPersistence: true})
	if err != nil {
		t.Fatal(err)
	}
	mgr.attach(client)
	mgr.pending[pending.ID] = &pendingPairing{id: pending.ID, client: client, status: PairingPending}

	if status, _, err := mgr.PairingQR(pending.ID); err != nil || status != PairingPending {
		t.Fatalf("PairingQR = %q, %v", status, err)
	}

	// whatsmeow sets the JID on the device just before dispatching PairSuccess.
	newJID := types.JID{User: testJIDB, Device: 2, Server: types.DefaultUserServer}
	client.Store.ID = &newJID
	mgr.onEvent(client, &events.PairSuccess{ID: newJID})

	if got := <-hooked; got != testJIDB+"@s.whatsapp.net" {
		t.Errorf("client hooks should run for the newly paired number, got %q", got)
	}
	inst, _ := store.GetInstance(pending.ID)
	if inst == nil || inst.PhoneJID != testJIDB+"@s.whatsapp.net" || inst.Status != database.InstanceStatusConnected {
		t.Errorf("pending row should be bound to the new number: %+v", inst)
	}
	if mgr.PairedCount() != 2 || mgr.PendingCount() != 0 {
		t.Errorf("pool after pairing: paired=%d pending=%d", mgr.PairedCount(), mgr.PendingCount())
	}
	if _, _, err := mgr.PairingQR(pending.ID); err != ErrInstanceNotFound {
		t.Errorf("a finished pairing is no longer pending, got %v", err)
	}
}

func TestPairingLimit(t *testing.T) {
	mgr, _ := newManagerEnv(t)
	mgr.cfg.MaxPendingPairings = 1
	mgr.pending[1] = &pendingPairing{id: 1}
	if _, err := mgr.CreatePairing("x", nil, false, nil); err != ErrTooManyPairings {
		t.Errorf("expected ErrTooManyPairings, got %v", err)
	}
}

func TestLoggedOutNumberLeavesThePoolOnlyWhenOthersRemain(t *testing.T) {
	mgr, store := newManagerEnv(t, testJIDA, testJIDB)
	a, _ := mgr.ClientByJID(testJIDA)

	mgr.onEvent(a, &events.LoggedOut{})
	if mgr.PairedCount() != 1 {
		t.Errorf("logged-out number should leave the pool, %d paired", mgr.PairedCount())
	}
	inst, _ := store.GetInstanceByPhone(testJIDA + "@s.whatsapp.net")
	if inst.Status != database.InstanceStatusLoggedOut {
		t.Errorf("status should be logged_out, got %q", inst.Status)
	}

	// A lone number stays so the pairing flow can start over.
	b, _ := mgr.ClientByJID(testJIDB)
	mgr.onEvent(b, &events.LoggedOut{})
	if len(mgr.ListClients()) != 1 {
		t.Errorf("the last client must stay in the pool, got %d", len(mgr.ListClients()))
	}
}

func TestManualDisconnectIsTrackedPerClient(t *testing.T) {
	mgr, store := newManagerEnv(t, testJIDA, testJIDB)
	inst, _ := store.GetInstanceByPhone(testJIDA + "@s.whatsapp.net")
	a, _ := mgr.ClientByJID(testJIDA)
	b, _ := mgr.ClientByJID(testJIDB)

	if err := mgr.DisconnectInstance(inst.ID); err != nil {
		t.Fatal(err)
	}
	if !mgr.IsManuallyDisconnected(a) || mgr.IsManuallyDisconnected(b) {
		t.Error("only the disconnected number should be marked")
	}
	if got, _ := store.GetInstance(inst.ID); got.Status != database.InstanceStatusDisconnected {
		t.Errorf("status should be disconnected, got %q", got.Status)
	}
}

func TestAntibanStateIsPerNumber(t *testing.T) {
	if antibanPathFor("5511900000002@s.whatsapp.net") != "store/antiban_warmup_5511900000002.json" {
		t.Error("unexpected per-number warm-up path")
	}
}

func TestHookContextIsCanceledWhenTheNumberIsRemoved(t *testing.T) {
	mgr, store := newManagerEnv(t, testJIDA, testJIDB)
	ctxs := make(chan context.Context, 2)
	mgr.AddClientHook(func(ctx context.Context, c *Client) { ctxs <- ctx })
	<-ctxs
	<-ctxs

	inst, _ := store.GetInstanceByPhone(testJIDA + "@s.whatsapp.net")
	entry := mgr.clients[testJIDA+"@s.whatsapp.net"]
	if err := mgr.RemoveInstance(inst.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entry.ctx.Done():
	case <-time.After(time.Second):
		t.Error("removing a number must cancel the goroutines started for it")
	}
	if got, _ := store.GetInstance(inst.ID); got.Status != database.InstanceStatusRemoved {
		t.Errorf("instance should be marked removed, got %q", got.Status)
	}
}
