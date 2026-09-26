package database

import (
	"testing"
)

func newTestOrgStore(t *testing.T) *MessageStore {
	t.Helper()
	return newTestStore(t)
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
