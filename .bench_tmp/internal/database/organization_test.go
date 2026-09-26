package database

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newTestOrgStore(t *testing.T) *MessageStore {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := createTables(db); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	store := &MessageStore{db: db}
	store.ensureWriter()
	t.Cleanup(func() { _ = store.Close() })

	return store
}

func TestDepartmentsCRUD(t *testing.T) {
	store := newTestOrgStore(t)

	// Create
	dept, err := store.CreateDepartment("Comercial", "Setor responsável pelas vendas e negociações")
	if err != nil {
		t.Fatalf("CreateDepartment: %v", err)
	}
	if dept.ID == 0 || dept.Name != "Comercial" {
		t.Errorf("Unexpected department: %+v", dept)
	}

	// Get
	got, err := store.GetDepartment(dept.ID)
	if err != nil {
		t.Fatalf("GetDepartment: %v", err)
	}
	if got == nil || got.Name != "Comercial" {
		t.Errorf("GetDepartment want 'Comercial', got %+v", got)
	}

	// Update
	err = store.UpdateDepartment(dept.ID, "Comercial & Vendas", "Setor de vendas expandido")
	if err != nil {
		t.Fatalf("UpdateDepartment: %v", err)
	}
	updated, _ := store.GetDepartment(dept.ID)
	if updated.Name != "Comercial & Vendas" {
		t.Errorf("Updated name want 'Comercial & Vendas', got %q", updated.Name)
	}

	// List
	_, _ = store.CreateDepartment("Financeiro", "Setor financeiro e contas")
	list, err := store.ListDepartments()
	if err != nil {
		t.Fatalf("ListDepartments: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListDepartments want 2, got %d", len(list))
	}

	// Delete
	err = store.DeleteDepartment(dept.ID)
	if err != nil {
		t.Fatalf("DeleteDepartment: %v", err)
	}
	deleted, _ := store.GetDepartment(dept.ID)
	if deleted != nil {
		t.Errorf("expected deleted department to be nil, got %+v", deleted)
	}
}

func TestEmployeesCRUDAndDesambiguation(t *testing.T) {
	store := newTestOrgStore(t)

	comercial, err := store.CreateDepartment("Comercial", "Vendas")
	if err != nil {
		t.Fatalf("CreateDepartment: %v", err)
	}
	financeiro, err := store.CreateDepartment("Financeiro", "Finanças")
	if err != nil {
		t.Fatalf("CreateDepartment: %v", err)
	}

	// Create two Joãos in different departments
	joaoVendedor, err := store.CreateEmployee(&comercial.ID, "João Silva", "Vendedor Pleno", "joao.vendas@empresa.com")
	if err != nil {
		t.Fatalf("CreateEmployee: %v", err)
	}
	if joaoVendedor.DepartmentName != "Comercial" {
		t.Errorf("expected department name 'Comercial', got %q", joaoVendedor.DepartmentName)
	}

	joaoFinanceiro, err := store.CreateEmployee(&financeiro.ID, "João Carlos", "Analista Financeiro", "joao.fin@empresa.com")
	if err != nil {
		t.Fatalf("CreateEmployee: %v", err)
	}
	if joaoFinanceiro.DepartmentName != "Financeiro" {
		t.Errorf("expected department name 'Financeiro', got %q", joaoFinanceiro.DepartmentName)
	}

	// AI Desambiguation test: Search by "joão"
	joaos, err := store.FindEmployeesByName("joão")
	if err != nil {
		t.Fatalf("FindEmployeesByName: %v", err)
	}
	if len(joaos) != 2 {
		t.Fatalf("expected 2 Joãos, got %d", len(joaos))
	}

	// Filter by department
	comercialTeam, err := store.ListEmployees(&comercial.ID)
	if err != nil {
		t.Fatalf("ListEmployees by dept: %v", err)
	}
	if len(comercialTeam) != 1 || comercialTeam[0].Role != "Vendedor Pleno" {
		t.Errorf("expected 1 vendedor pleno in comercial, got %+v", comercialTeam)
	}
}

func TestInstancesCRUDAndMessageTagging(t *testing.T) {
	store := newTestOrgStore(t)

	comercial, _ := store.CreateDepartment("Comercial", "Vendas")
	joao, _ := store.CreateEmployee(&comercial.ID, "João Silva", "Vendedor", "joao@empresa.com")

	phoneJID := "5511999990001@s.whatsapp.net"
	inst, err := store.UpsertInstance(phoneJID, &joao.ID, "Celular Vendas 01", "connected")
	if err != nil {
		t.Fatalf("UpsertInstance: %v", err)
	}
	if inst.EmployeeName != "João Silva" {
		t.Errorf("expected EmployeeName 'João Silva', got %q", inst.EmployeeName)
	}

	// Update status
	now := time.Now()
	err = store.UpdateInstanceStatus(phoneJID, "disconnected", nil, &now)
	if err != nil {
		t.Fatalf("UpdateInstanceStatus: %v", err)
	}

	gotInst, err := store.GetInstanceByPhone(phoneJID)
	if err != nil {
		t.Fatalf("GetInstanceByPhone: %v", err)
	}
	if gotInst.Status != "disconnected" {
		t.Errorf("expected status 'disconnected', got %q", gotInst.Status)
	}

	// Store chat first to satisfy foreign key constraint on messages(chat_jid)
	chatJID := "5511888880000@s.whatsapp.net"
	if err := store.StoreChatWithInstance(chatJID, "Cliente Fulano", time.Now(), phoneJID); err != nil {
		t.Fatalf("StoreChatWithInstance: %v", err)
	}

	// Store message tagged with instance_jid
	msgID := "MSG_TEST_001"
	err = store.StoreMessageWithInstance(
		msgID, chatJID, "cliente", "Cliente Fulano", "Gostaria de fechar o pedido de R$ 5.000",
		time.Now(), false, "", "", "", "", nil, nil, nil, 0, phoneJID, false,
	)
	if err != nil {
		t.Fatalf("StoreMessageWithInstance: %v", err)
	}

	// Verify message in db has instance_jid
	var storedInstanceJID string
	var isDeleted bool
	err = store.db.QueryRow("SELECT instance_jid, is_deleted_remote FROM messages WHERE id = ?", msgID).Scan(&storedInstanceJID, &isDeleted)
	if err != nil {
		t.Fatalf("query message: %v", err)
	}
	if storedInstanceJID != phoneJID {
		t.Errorf("expected instance_jid %q, got %q", phoneJID, storedInstanceJID)
	}
	if isDeleted {
		t.Errorf("expected is_deleted_remote to be false initially")
	}

	// Test Anti-delete: mark remote deleted
	err = store.MarkMessageRemoteDeleted(msgID, chatJID)
	if err != nil {
		t.Fatalf("MarkMessageRemoteDeleted: %v", err)
	}

	err = store.db.QueryRow("SELECT is_deleted_remote FROM messages WHERE id = ?", msgID).Scan(&isDeleted)
	if err != nil {
		t.Fatalf("query message after delete: %v", err)
	}
	if !isDeleted {
		t.Errorf("expected is_deleted_remote to be true after marking deleted")
	}
}
