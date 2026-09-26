package database

import (
	"database/sql"
	"errors"
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
			return ErrNotFound
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

// UpdateEmployee updates employee data. It returns ErrNotFound when the employee does not exist.
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
			return ErrNotFound
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
// INSTANCES
// ============================================================================

// ErrNotFound is returned by update operations that match no row.
var ErrNotFound = errors.New("record not found")

// Instance statuses. "pairing" rows have no phone_jid yet; "removed" rows keep the number
// so messages already captured stay attributed to it.
const (
	InstanceStatusPairing      = "pairing"
	InstanceStatusConnected    = "connected"
	InstanceStatusDisconnected = "disconnected"
	InstanceStatusLoggedOut    = "logged_out"
	InstanceStatusRemoved      = "removed"
)

// CorporateConfirmation records that a responsible person attested the number is a company asset.
type CorporateConfirmation struct {
	TermsVersion string
	ConfirmedBy  string
}

// InstanceUpdate carries the editable fields of an instance. Nil pointers mean "leave as is";
// SetEmployee distinguishes "unlink the employee" (EmployeeID nil) from "not provided".
type InstanceUpdate struct {
	Alias       *string
	SetEmployee bool
	EmployeeID  *int
	AllowSend   *bool
	Confirm     *CorporateConfirmation
}

const instanceSelect = `
	SELECT i.id, COALESCE(i.phone_jid, ''), i.employee_id, COALESCE(e.name, ''), e.department_id, COALESCE(d.name, ''),
	       COALESCE(i.alias, ''), COALESCE(i.status, 'disconnected'), i.paired_at, i.last_seen_at,
	       i.allow_send, i.corporate_asset_confirmed, COALESCE(i.corporate_terms_version, ''),
	       COALESCE(i.corporate_confirmed_by, ''), i.corporate_confirmed_at, i.created_at, i.updated_at
	FROM instances i
	LEFT JOIN employees e ON e.id = i.employee_id
	LEFT JOIN departments d ON d.id = e.department_id
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInstance(row rowScanner) (*types.Instance, error) {
	inst := &types.Instance{}
	if err := row.Scan(&inst.ID, &inst.PhoneJID, &inst.EmployeeID, &inst.EmployeeName, &inst.DepartmentID, &inst.DepartmentName,
		&inst.Alias, &inst.Status, &inst.PairedAt, &inst.LastSeenAt,
		&inst.AllowSend, &inst.CorporateAssetConfirmed, &inst.CorporateTermsVersion,
		&inst.CorporateConfirmedBy, &inst.CorporateConfirmedAt, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
		return nil, err
	}
	if inst.PhoneJID != "" {
		inst.PhoneNumber = strings.SplitN(inst.PhoneJID, "@", 2)[0]
	}
	return inst, nil
}

// GetInstance retrieves an instance by its id.
func (store *MessageStore) GetInstance(id int) (*types.Instance, error) {
	inst, err := scanInstance(store.db.QueryRow(instanceSelect+` WHERE i.id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %d: %w", id, err)
	}
	return inst, nil
}

// GetInstanceByPhone retrieves an instance by its WhatsApp phone JID.
func (store *MessageStore) GetInstanceByPhone(phoneJID string) (*types.Instance, error) {
	inst, err := scanInstance(store.db.QueryRow(instanceSelect+` WHERE i.phone_jid = ?`, phoneJID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s: %w", phoneJID, err)
	}
	return inst, nil
}

// ListInstances returns the registered instances. Removed instances are omitted unless requested.
func (store *MessageStore) ListInstances(includeRemoved bool) ([]*types.Instance, error) {
	query := instanceSelect
	if !includeRemoved {
		query += ` WHERE COALESCE(i.status, '') != 'removed'`
	}
	rows, err := store.db.Query(query + ` ORDER BY i.created_at ASC, i.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	defer rows.Close()

	var list []*types.Instance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, inst)
	}
	return list, rows.Err()
}

// assignEmployeeTx makes employeeID the current owner of the instance, closing the previous
// assignment so history stays attributable. It is a no-op when the owner does not change.
func assignEmployeeTx(tx *sql.Tx, instanceID int, employeeID *int, at time.Time) error {
	var current sql.NullInt64
	err := tx.QueryRow(`SELECT employee_id FROM instances WHERE id = ?`, instanceID).Scan(&current)
	if err != nil {
		return err
	}
	if (employeeID == nil && !current.Valid) || (employeeID != nil && current.Valid && int64(*employeeID) == current.Int64) {
		return nil
	}

	if _, err := tx.Exec(`UPDATE instance_assignments SET valid_to = ? WHERE instance_id = ? AND valid_to IS NULL`, at, instanceID); err != nil {
		return err
	}
	if employeeID != nil {
		if _, err := tx.Exec(`INSERT INTO instance_assignments (instance_id, employee_id, valid_from) VALUES (?, ?, ?)`, instanceID, *employeeID, at); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE instances SET employee_id = ?, updated_at = ? WHERE id = ?`, employeeID, at, instanceID)
	return err
}

// CreatePendingInstance registers a number that is about to be paired. It has no JID until the
// QR code is scanned.
func (store *MessageStore) CreatePendingInstance(alias string, employeeID *int, allowSend bool, confirm *CorporateConfirmation) (*types.Instance, error) {
	now := time.Now().UTC()
	var id int
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO instances (phone_jid, alias, status, allow_send, created_at, updated_at) VALUES (NULL, ?, ?, ?, ?, ?)`,
			alias, InstanceStatusPairing, allowSend, now, now)
		if err != nil {
			return err
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		id = int(newID)
		if confirm != nil {
			if _, err := tx.Exec(`UPDATE instances SET corporate_asset_confirmed = 1, corporate_terms_version = ?, corporate_confirmed_by = ?, corporate_confirmed_at = ? WHERE id = ?`,
				confirm.TermsVersion, confirm.ConfirmedBy, now, id); err != nil {
				return err
			}
		}
		if employeeID != nil {
			return assignEmployeeTx(tx, id, employeeID, now)
		}
		return nil
	}, false, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create pending instance: %w", err)
	}
	return store.GetInstance(id)
}

// RegisterInstanceJID ensures an instance row exists for a paired number (devices loaded from the
// session store, or the first device paired through the legacy flow). Existing rows, including
// removed ones, are reused so their history stays attached.
func (store *MessageStore) RegisterInstanceJID(phoneJID string, defaultAllowSend bool) (*types.Instance, error) {
	now := time.Now().UTC()
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT OR IGNORE INTO instances (phone_jid, status, allow_send, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			phoneJID, InstanceStatusDisconnected, defaultAllowSend, now, now)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE instances SET status = ?, updated_at = ? WHERE phone_jid = ? AND status IN (?, ?)`,
			InstanceStatusDisconnected, now, phoneJID, InstanceStatusRemoved, InstanceStatusLoggedOut)
		return err
	}, false, true)
	if err != nil {
		return nil, fmt.Errorf("failed to register instance %s: %w", phoneJID, err)
	}
	return store.GetInstanceByPhone(phoneJID)
}

// CompleteInstancePairing binds a pending instance to the JID that was just paired. When the
// number already has a row (the same phone paired again) the pending row is folded into it.
func (store *MessageStore) CompleteInstancePairing(id int, phoneJID string) (*types.Instance, error) {
	now := time.Now().UTC()
	var finalID int
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		var existingID int
		err := tx.QueryRow(`SELECT id FROM instances WHERE phone_jid = ? AND id != ?`, phoneJID, id).Scan(&existingID)
		switch {
		case err == sql.ErrNoRows:
			finalID = id
			_, err = tx.Exec(`UPDATE instances SET phone_jid = ?, status = ?, paired_at = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
				phoneJID, InstanceStatusConnected, now, now, now, id)
			return err
		case err != nil:
			return err
		}

		// Same number paired again: keep the older row, carry over what the operator just entered.
		finalID = existingID
		var alias sql.NullString
		var employee sql.NullInt64
		if err := tx.QueryRow(`SELECT alias, employee_id FROM instances WHERE id = ?`, id).Scan(&alias, &employee); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE instances SET status = ?, paired_at = ?, last_seen_at = ?, updated_at = ?,
			alias = CASE WHEN ? != '' THEN ? ELSE alias END WHERE id = ?`,
			InstanceStatusConnected, now, now, now, alias.String, alias.String, existingID); err != nil {
			return err
		}
		if employee.Valid {
			emp := int(employee.Int64)
			if err := assignEmployeeTx(tx, existingID, &emp, now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`DELETE FROM instance_assignments WHERE instance_id = ?`, id); err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM instances WHERE id = ?`, id)
		return err
	}, false, true)
	if err != nil {
		return nil, fmt.Errorf("failed to complete pairing for instance %d: %w", id, err)
	}
	return store.GetInstance(finalID)
}

// SetInstanceStatus updates the connection status of a paired instance. lastSeen refreshes last_seen_at.
func (store *MessageStore) SetInstanceStatus(phoneJID, status string, lastSeen bool) error {
	now := time.Now().UTC()
	return store.enqueueWrite(func(tx *sql.Tx) error {
		if lastSeen {
			_, err := tx.Exec(`UPDATE instances SET status = ?, last_seen_at = ?, updated_at = ? WHERE phone_jid = ?`, status, now, now, phoneJID)
			return err
		}
		_, err := tx.Exec(`UPDATE instances SET status = ?, updated_at = ? WHERE phone_jid = ?`, status, now, phoneJID)
		return err
	}, false, true)
}

// UpdateInstance applies editable fields. It returns ErrNotFound when the instance does not exist.
func (store *MessageStore) UpdateInstance(id int, upd InstanceUpdate) (*types.Instance, error) {
	now := time.Now().UTC()
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM instances WHERE id = ?`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		if upd.Alias != nil {
			if _, err := tx.Exec(`UPDATE instances SET alias = ?, updated_at = ? WHERE id = ?`, *upd.Alias, now, id); err != nil {
				return err
			}
		}
		if upd.AllowSend != nil {
			if _, err := tx.Exec(`UPDATE instances SET allow_send = ?, updated_at = ? WHERE id = ?`, *upd.AllowSend, now, id); err != nil {
				return err
			}
		}
		if upd.Confirm != nil {
			if _, err := tx.Exec(`UPDATE instances SET corporate_asset_confirmed = 1, corporate_terms_version = ?, corporate_confirmed_by = ?, corporate_confirmed_at = ?, updated_at = ? WHERE id = ?`,
				upd.Confirm.TermsVersion, upd.Confirm.ConfirmedBy, now, now, id); err != nil {
				return err
			}
		}
		if upd.SetEmployee {
			return assignEmployeeTx(tx, id, upd.EmployeeID, now)
		}
		return nil
	}, false, true)
	if err != nil {
		return nil, err
	}
	return store.GetInstance(id)
}

// MarkInstanceRemoved retires an instance without deleting it: its credentials are gone but the
// messages it captured stay attributed to it.
func (store *MessageStore) MarkInstanceRemoved(id int) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		now := time.Now().UTC()
		if _, err := tx.Exec(`UPDATE instance_assignments SET valid_to = ? WHERE instance_id = ? AND valid_to IS NULL`, now, id); err != nil {
			return err
		}
		res, err := tx.Exec(`UPDATE instances SET status = ?, updated_at = ? WHERE id = ?`, InstanceStatusRemoved, now, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	}, false, true)
}

// DeletePendingInstance removes a row that never finished pairing.
func (store *MessageStore) DeletePendingInstance(id int) error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM instance_assignments WHERE instance_id IN (SELECT id FROM instances WHERE id = ? AND phone_jid IS NULL)`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM instances WHERE id = ? AND phone_jid IS NULL`, id)
		return err
	}, false, true)
}

// PurgeStalePendingInstances removes pairing rows left behind by a previous run.
func (store *MessageStore) PurgeStalePendingInstances() error {
	return store.enqueueWrite(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM instance_assignments WHERE instance_id IN (SELECT id FROM instances WHERE phone_jid IS NULL)`); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM instances WHERE phone_jid IS NULL`)
		return err
	}, false, true)
}

// InstanceMessageCount returns how many captured messages are attributed to the instance.
func (store *MessageStore) InstanceMessageCount(phoneJID string) (int, error) {
	var n int
	err := store.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE instance_jid = ?`, phoneJID).Scan(&n)
	return n, err
}

// InstanceAllowsSend reports whether outbound actions are enabled for the instance.
func (store *MessageStore) InstanceAllowsSend(phoneJID string) (bool, error) {
	var allowed bool
	err := store.db.QueryRow(`SELECT allow_send FROM instances WHERE phone_jid = ?`, phoneJID).Scan(&allowed)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return allowed, err
}

// SetRequireCorporateConfirmation makes message capture skip instances whose number has not been
// confirmed as a corporate asset.
func (store *MessageStore) SetRequireCorporateConfirmation(required bool) {
	store.requireCorporateConfirmation.Store(required)
}

// CaptureAllowed reports whether messages from the instance may be stored.
func (store *MessageStore) CaptureAllowed(phoneJID string) bool {
	if !store.requireCorporateConfirmation.Load() {
		return true
	}
	var confirmed bool
	err := store.db.QueryRow(`SELECT corporate_asset_confirmed FROM instances WHERE phone_jid = ?`, phoneJID).Scan(&confirmed)
	return err == nil && confirmed
}

// BackfillLegacyInstance attributes messages captured before instances existed (empty
// instance_jid) to the given number. Their rowids move past the current maximum so the search
// indexer picks them up again with the new attribution.
func (store *MessageStore) BackfillLegacyInstance(phoneJID string) (int64, error) {
	var moved int64
	err := store.enqueueWrite(func(tx *sql.Tx) error {
		var maxRowID int64
		if err := tx.QueryRow(`SELECT COALESCE(MAX(rowid), 0) FROM messages`).Scan(&maxRowID); err != nil {
			return err
		}
		res, err := tx.Exec(`UPDATE messages SET instance_jid = ?, rowid = rowid + ? WHERE instance_jid = ''`, phoneJID, maxRowID)
		if err != nil {
			return err
		}
		moved, _ = res.RowsAffected()
		if moved == 0 {
			return nil
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO chat_instances (instance_jid, chat_jid, last_message_time)
			SELECT instance_jid, chat_jid, MAX(timestamp) FROM messages WHERE instance_jid = ? GROUP BY instance_jid, chat_jid`, phoneJID); err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM chat_instances WHERE instance_jid = ''`)
		return err
	}, false, true)
	return moved, err
}

// CountUnattributedMessages returns how many messages carry no instance.
func (store *MessageStore) CountUnattributedMessages() (int, error) {
	var n int
	err := store.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE instance_jid = ''`).Scan(&n)
	return n, err
}
