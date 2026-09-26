package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"whatsapp-bridge/internal/types"
)

// ============================================================================
// DEPARTMENTS CRUD
// ============================================================================

// CreateDepartment creates a new department entity in the organization.
func (store *MessageStore) CreateDepartment(name, description string) (*types.Department, error) {
	var createdID int
	now := time.Now()

	err := store.enqueueWrite(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`INSERT INTO departments (name, description, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			name, description, now, now,
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		createdID = int(id)
		return nil
	}, false, true)

	if err != nil {
		return nil, fmt.Errorf("failed to create department: %w", err)
	}

	return &types.Department{
		ID:          createdID,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetDepartment retrieves a department by ID.
func (store *MessageStore) GetDepartment(id int) (*types.Department, error) {
	row := store.db.QueryRow(
		`SELECT id, name, COALESCE(description, ''), created_at, updated_at FROM departments WHERE id = ?`,
		id,
	)

	dept := &types.Department{}
	err := row.Scan(&dept.ID, &dept.Name, &dept.Description, &dept.CreatedAt, &dept.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get department %d: %w", id, err)
	}

	return dept, nil
}

// ListDepartments returns all registered departments.
func (store *MessageStore) ListDepartments() ([]*types.Department, error) {
	rows, err := store.db.Query(`SELECT id, name, COALESCE(description, ''), created_at, updated_at FROM departments ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list departments: %w", err)
	}
	defer rows.Close()

	var list []*types.Department
	for rows.Next() {
		dept := &types.Department{}
		if err := rows.Scan(&dept.ID, &dept.Name, &dept.Description, &dept.CreatedAt, &dept.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, dept)
	}

	return list, rows.Err()
}

// UpdateDepartment updates an existing department's name and description.
func (store *MessageStore) UpdateDepartment(id int, name, description string) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`UPDATE departments SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
			name, description, time.Now(), id,
		)
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return sql.ErrNoRows
		}
		return nil
	}, false, true)
}

// DeleteDepartment removes a department (setting employee department_ids to NULL).
func (store *MessageStore) DeleteDepartment(id int) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM departments WHERE id = ?`, id)
		return err
	}, false, true)
}

// ============================================================================
// EMPLOYEES CRUD
// ============================================================================

// CreateEmployee creates a new employee entity linked to a department.
func (store *MessageStore) CreateEmployee(departmentID *int, name, role, email string) (*types.Employee, error) {
	var createdID int
	now := time.Now()

	err := store.enqueueWrite(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`INSERT INTO employees (department_id, name, role, email, active, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`,
			departmentID, name, role, email, now, now,
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		createdID = int(id)
		return nil
	}, false, true)

	if err != nil {
		return nil, fmt.Errorf("failed to create employee: %w", err)
	}

	return store.GetEmployee(createdID)
}

// GetEmployee retrieves an employee by ID including department name.
func (store *MessageStore) GetEmployee(id int) (*types.Employee, error) {
	row := store.db.QueryRow(`
		SELECT e.id, e.department_id, COALESCE(d.name, ''), e.name, COALESCE(e.role, ''), COALESCE(e.email, ''), e.active, e.created_at, e.updated_at
		FROM employees e
		LEFT JOIN departments d ON d.id = e.department_id
		WHERE e.id = ?
	`, id)

	emp := &types.Employee{}
	err := row.Scan(&emp.ID, &emp.DepartmentID, &emp.DepartmentName, &emp.Name, &emp.Role, &emp.Email, &emp.Active, &emp.CreatedAt, &emp.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get employee %d: %w", id, err)
	}

	return emp, nil
}

// ListEmployees returns all employees, optionally filtered by department.
func (store *MessageStore) ListEmployees(departmentID *int) ([]*types.Employee, error) {
	query := `
		SELECT e.id, e.department_id, COALESCE(d.name, ''), e.name, COALESCE(e.role, ''), COALESCE(e.email, ''), e.active, e.created_at, e.updated_at
		FROM employees e
		LEFT JOIN departments d ON d.id = e.department_id
	`
	var args []interface{}
	if departmentID != nil {
		query += " WHERE e.department_id = ?"
		args = append(args, *departmentID)
	}
	query += " ORDER BY e.name ASC"

	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list employees: %w", err)
	}
	defer rows.Close()

	var list []*types.Employee
	for rows.Next() {
		emp := &types.Employee{}
		if err := rows.Scan(&emp.ID, &emp.DepartmentID, &emp.DepartmentName, &emp.Name, &emp.Role, &emp.Email, &emp.Active, &emp.CreatedAt, &emp.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, emp)
	}

	return list, rows.Err()
}

// FindEmployeesByName searches employees by partial name match (case-insensitive) for AI desambiguation.
func (store *MessageStore) FindEmployeesByName(nameQuery string) ([]*types.Employee, error) {
	likePattern := "%" + strings.ToLower(strings.TrimSpace(nameQuery)) + "%"
	rows, err := store.db.Query(`
		SELECT e.id, e.department_id, COALESCE(d.name, ''), e.name, COALESCE(e.role, ''), COALESCE(e.email, ''), e.active, e.created_at, e.updated_at
		FROM employees e
		LEFT JOIN departments d ON d.id = e.department_id
		WHERE LOWER(e.name) LIKE ?
		ORDER BY e.name ASC
	`, likePattern)
	if err != nil {
		return nil, fmt.Errorf("failed to find employees by name: %w", err)
	}
	defer rows.Close()

	var list []*types.Employee
	for rows.Next() {
		emp := &types.Employee{}
		if err := rows.Scan(&emp.ID, &emp.DepartmentID, &emp.DepartmentName, &emp.Name, &emp.Role, &emp.Email, &emp.Active, &emp.CreatedAt, &emp.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, emp)
	}

	return list, rows.Err()
}

// UpdateEmployee updates employee data.
func (store *MessageStore) UpdateEmployee(id int, departmentID *int, name, role, email string, active bool) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		res, err := tx.Exec(
			`UPDATE employees SET department_id = ?, name = ?, role = ?, email = ?, active = ?, updated_at = ? WHERE id = ?`,
			departmentID, name, role, email, active, time.Now(), id,
		)
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return sql.ErrNoRows
		}
		return nil
	}, false, true)
}

// DeleteEmployee deletes an employee.
func (store *MessageStore) DeleteEmployee(id int) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM employees WHERE id = ?`, id)
		return err
	}, false, true)
}

// ============================================================================
// INSTANCES CRUD
// ============================================================================

// UpsertInstance registers or updates an instance connection.
func (store *MessageStore) UpsertInstance(phoneJID string, employeeID *int, alias, status string) (*types.Instance, error) {
	now := time.Now()
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO instances (phone_jid, employee_id, alias, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(phone_jid) DO UPDATE SET
				employee_id = CASE WHEN excluded.employee_id IS NOT NULL THEN excluded.employee_id ELSE instances.employee_id END,
				alias = CASE WHEN excluded.alias != '' THEN excluded.alias ELSE instances.alias END,
				status = CASE WHEN excluded.status != '' THEN excluded.status ELSE instances.status END,
				updated_at = excluded.updated_at
		`, phoneJID, employeeID, alias, status, now, now)
		return err
	}, false, true)

	if err != nil {
		return nil, fmt.Errorf("failed to upsert instance %s: %w", phoneJID, err)
	}

	return store.GetInstanceByPhone(phoneJID)
}

// GetInstanceByPhone retrieves an instance by its WhatsApp phone JID.
func (store *MessageStore) GetInstanceByPhone(phoneJID string) (*types.Instance, error) {
	row := store.db.QueryRow(`
		SELECT i.id, i.phone_jid, i.employee_id, COALESCE(e.name, ''), COALESCE(i.alias, ''), i.status, i.paired_at, i.last_seen_at, i.created_at, i.updated_at
		FROM instances i
		LEFT JOIN employees e ON e.id = i.employee_id
		WHERE i.phone_jid = ?
	`, phoneJID)

	inst := &types.Instance{}
	err := row.Scan(&inst.ID, &inst.PhoneJID, &inst.EmployeeID, &inst.EmployeeName, &inst.Alias, &inst.Status, &inst.PairedAt, &inst.LastSeenAt, &inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get instance %s: %w", phoneJID, err)
	}

	return inst, nil
}

// ListInstances returns all registered WhatsApp instances.
func (store *MessageStore) ListInstances() ([]*types.Instance, error) {
	rows, err := store.db.Query(`
		SELECT i.id, i.phone_jid, i.employee_id, COALESCE(e.name, ''), COALESCE(i.alias, ''), i.status, i.paired_at, i.last_seen_at, i.created_at, i.updated_at
		FROM instances i
		LEFT JOIN employees e ON e.id = i.employee_id
		ORDER BY i.created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	defer rows.Close()

	var list []*types.Instance
	for rows.Next() {
		inst := &types.Instance{}
		if err := rows.Scan(&inst.ID, &inst.PhoneJID, &inst.EmployeeID, &inst.EmployeeName, &inst.Alias, &inst.Status, &inst.PairedAt, &inst.LastSeenAt, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, inst)
	}

	return list, rows.Err()
}

// UpdateInstanceStatus updates the connection status and timestamps of an instance.
func (store *MessageStore) UpdateInstanceStatus(phoneJID, status string, pairedAt, lastSeenAt *time.Time) error {
	now := time.Now()
	return store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			UPDATE instances SET
				status = ?,
				paired_at = COALESCE(?, paired_at),
				last_seen_at = COALESCE(?, last_seen_at),
				updated_at = ?
			WHERE phone_jid = ?
		`, status, pairedAt, lastSeenAt, now, phoneJID)
		return err
	}, false, true)
}

// DeleteInstance removes an instance record.
func (store *MessageStore) DeleteInstance(phoneJID string) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM instances WHERE phone_jid = ?`, phoneJID)
		return err
	}, false, true)
}

// ============================================================================
// AUDIT & MESSAGE TAGGING
// ============================================================================

// MarkMessageRemoteDeleted flags a message as revoked/deleted by the remote sender without deleting physical record.
func (store *MessageStore) MarkMessageRemoteDeleted(messageID, chatJID string) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`UPDATE messages SET is_deleted_remote = 1 WHERE id = ? AND chat_jid = ?`,
			messageID, chatJID,
		)
		return err
	}, false, true)
}
