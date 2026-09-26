package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"whatsapp-bridge/internal/database"
	"whatsapp-bridge/internal/types"
	"whatsapp-bridge/internal/whatsapp"
)

// currentTermsVersion identifies the wording of the corporate-asset attestation the panel shows.
// Bump it when the text changes so earlier confirmations stay traceable to what was agreed.
const currentTermsVersion = "2026-09"

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) fail(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]interface{}{"success": false, "error": msg})
}

// serverError logs the real cause and returns a generic message: database errors can carry
// table and column names that callers have no use for.
func (s *Server) serverError(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, database.ErrNotFound) {
		s.fail(w, http.StatusNotFound, "not found")
		return
	}
	log.Printf("api: %s: %v", what, err)
	s.fail(w, http.StatusInternalServerError, "internal error")
}

// decodeFields reads a JSON object keeping which keys were present, so "field omitted" and
// "field set to null" can be told apart in partial updates.
func decodeFields(r *http.Request) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		return nil, err
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	return fields, nil
}

func fieldString(f map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := f[key]
	if !ok {
		return "", false
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	return v, true
}

func fieldBool(f map[string]json.RawMessage, key string) (bool, bool) {
	raw, ok := f[key]
	if !ok {
		return false, false
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, false
	}
	return v, true
}

// fieldOptionalInt returns (value, present). A JSON null yields (nil, true).
func fieldOptionalInt(f map[string]json.RawMessage, key string) (*int, bool) {
	raw, ok := f[key]
	if !ok {
		return nil, false
	}
	var v *int
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false
	}
	return v, true
}

// ============================================================================
// DEPARTMENTS
// ============================================================================

// handleDepartments handles GET (list) and POST (create) for departments.
func (s *Server) handleDepartments(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		list, err := s.messageStore.ListDepartments()
		if err != nil {
			s.serverError(w, "list departments", err)
			return
		}
		if list == nil {
			list = []*types.Department{}
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "departments": list})

	case http.MethodPost:
		var body struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			s.fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			s.fail(w, http.StatusBadRequest, "name is required")
			return
		}

		dept, err := s.messageStore.CreateDepartment(strings.TrimSpace(body.Name), body.Description)
		if err != nil {
			s.serverError(w, "create department", err)
			return
		}
		s.writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "department": dept})

	default:
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleDepartmentByID handles GET, PUT, and DELETE for a specific department.
func (s *Server) handleDepartmentByID(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.Atoi(strings.TrimPrefix(req.URL.Path, "/api/departments/"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, "invalid department id")
		return
	}

	switch req.Method {
	case http.MethodGet:
		dept, err := s.messageStore.GetDepartment(id)
		if err != nil {
			s.serverError(w, "get department", err)
			return
		}
		if dept == nil {
			s.fail(w, http.StatusNotFound, "department not found")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "department": dept})

	case http.MethodPut:
		var body struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			s.fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			s.fail(w, http.StatusBadRequest, "name is required")
			return
		}
		if err := s.messageStore.UpdateDepartment(id, strings.TrimSpace(body.Name), body.Description); err != nil {
			s.serverError(w, "update department", err)
			return
		}
		dept, _ := s.messageStore.GetDepartment(id)
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "department": dept})

	case http.MethodDelete:
		if err := s.messageStore.DeleteDepartment(id); err != nil {
			s.serverError(w, "delete department", err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "department deleted"})

	default:
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ============================================================================
// EMPLOYEES
// ============================================================================

// handleEmployees handles GET (list/search) and POST (create) for employees.
func (s *Server) handleEmployees(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		var list []*types.Employee
		var err error
		if q := req.URL.Query().Get("q"); q != "" {
			list, err = s.messageStore.FindEmployeesByName(q)
		} else {
			var deptID *int
			if dStr := req.URL.Query().Get("department_id"); dStr != "" {
				if parsed, convErr := strconv.Atoi(dStr); convErr == nil {
					deptID = &parsed
				}
			}
			list, err = s.messageStore.ListEmployees(deptID)
		}
		if err != nil {
			s.serverError(w, "list employees", err)
			return
		}
		if list == nil {
			list = []*types.Employee{}
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "employees": list})

	case http.MethodPost:
		var body struct {
			DepartmentID *int   `json:"department_id"`
			Name         string `json:"name"`
			Role         string `json:"role"`
			Email        string `json:"email"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			s.fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			s.fail(w, http.StatusBadRequest, "name is required")
			return
		}
		emp, err := s.messageStore.CreateEmployee(body.DepartmentID, strings.TrimSpace(body.Name), body.Role, body.Email)
		if err != nil {
			s.serverError(w, "create employee", err)
			return
		}
		s.writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "employee": emp})

	default:
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleEmployeeByID handles GET, PUT (partial), and DELETE for a specific employee.
func (s *Server) handleEmployeeByID(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.Atoi(strings.TrimPrefix(req.URL.Path, "/api/employees/"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, "invalid employee id")
		return
	}

	switch req.Method {
	case http.MethodGet:
		emp, err := s.messageStore.GetEmployee(id)
		if err != nil {
			s.serverError(w, "get employee", err)
			return
		}
		if emp == nil {
			s.fail(w, http.StatusNotFound, "employee not found")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "employee": emp})

	case http.MethodPut:
		current, err := s.messageStore.GetEmployee(id)
		if err != nil {
			s.serverError(w, "get employee", err)
			return
		}
		if current == nil {
			s.fail(w, http.StatusNotFound, "employee not found")
			return
		}
		fields, err := decodeFields(req)
		if err != nil {
			s.fail(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Partial update: only the fields present in the request change.
		name, role, email, active, dept := current.Name, current.Role, current.Email, current.Active, current.DepartmentID
		if v, ok := fieldString(fields, "name"); ok {
			if strings.TrimSpace(v) == "" {
				s.fail(w, http.StatusBadRequest, "name is required")
				return
			}
			name = strings.TrimSpace(v)
		}
		if v, ok := fieldString(fields, "role"); ok {
			role = v
		}
		if v, ok := fieldString(fields, "email"); ok {
			email = v
		}
		if v, ok := fieldBool(fields, "active"); ok {
			active = v
		}
		if v, ok := fieldOptionalInt(fields, "department_id"); ok {
			dept = v
		}

		if err := s.messageStore.UpdateEmployee(id, dept, name, role, email, active); err != nil {
			s.serverError(w, "update employee", err)
			return
		}
		emp, _ := s.messageStore.GetEmployee(id)
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "employee": emp})

	case http.MethodDelete:
		if err := s.messageStore.DeleteEmployee(id); err != nil {
			s.serverError(w, "delete employee", err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "employee deleted"})

	default:
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ============================================================================
// INSTANCES
// ============================================================================

// decorate fills the live connection state, which the database only knows as the last recorded status.
func (s *Server) decorate(inst *types.Instance) *types.Instance {
	if inst != nil && inst.PhoneJID != "" && s.instanceManager != nil {
		if c, err := s.instanceManager.ClientByJID(inst.PhoneJID); err == nil {
			inst.Live = c.IsConnected()
		}
	}
	return inst
}

// handleInstances handles GET (list) and POST (start pairing a new number).
func (s *Server) handleInstances(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		list, err := s.messageStore.ListInstances(req.URL.Query().Get("include_removed") == "true")
		if err != nil {
			s.serverError(w, "list instances", err)
			return
		}
		if list == nil {
			list = []*types.Instance{}
		}
		for _, inst := range list {
			s.decorate(inst)
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "instances": list})

	case http.MethodPost:
		s.handleCreateInstancePair(w, req)

	default:
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleCreateInstancePair starts the QR pairing of a new number. The request must carry the
// corporate-asset attestation: the panel shows the terms and sends corporate_asset_confirmed.
// The response has the instance (status "pairing") and, when it is ready in time, the first QR
// code; poll GET /api/instances/{id}/qr for later ones.
func (s *Server) handleCreateInstancePair(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.instanceManager == nil {
		s.fail(w, http.StatusServiceUnavailable, "multi-instance manager not initialized")
		return
	}

	fields, err := decodeFields(req)
	if err != nil {
		s.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	confirmed, _ := fieldBool(fields, "corporate_asset_confirmed")
	if !confirmed {
		s.fail(w, http.StatusBadRequest, "corporate_asset_confirmed must be true: confirm that this number is a company asset")
		return
	}
	alias, _ := fieldString(fields, "alias")
	employeeID, _ := fieldOptionalInt(fields, "employee_id")
	allowSend, _ := fieldBool(fields, "allow_send")

	inst, err := s.instanceManager.CreatePairing(strings.TrimSpace(alias), employeeID, allowSend, &database.CorporateConfirmation{
		TermsVersion: currentTermsVersion,
		ConfirmedBy:  actorOf(req),
	})
	switch {
	case errors.Is(err, whatsapp.ErrTooManyPairings):
		s.fail(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		s.serverError(w, "create pairing", err)
		return
	}

	// The first QR code arrives a moment after connecting.
	var qr string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, code, qrErr := s.instanceManager.PairingQR(inst.ID); qrErr == nil && code != "" {
			qr = code
			break
		}
		time.Sleep(150 * time.Millisecond)
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"instance": inst,
		"qr_code":  qr,
		"message":  "Instance created. Scan the QR code to complete pairing.",
	})
}

// handleInstanceByID serves /api/instances/{id} and its actions (qr, reconnect, disconnect).
func (s *Server) handleInstanceByID(w http.ResponseWriter, req *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/instances/"), "/"), "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		s.fail(w, http.StatusBadRequest, "invalid instance id")
		return
	}

	if len(parts) > 1 {
		s.handleInstanceAction(w, req, id, parts[1])
		return
	}

	switch req.Method {
	case http.MethodGet:
		inst, err := s.messageStore.GetInstance(id)
		if err != nil {
			s.serverError(w, "get instance", err)
			return
		}
		if inst == nil {
			s.fail(w, http.StatusNotFound, "instance not found")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "instance": s.decorate(inst)})

	case http.MethodPut, http.MethodPatch:
		fields, err := decodeFields(req)
		if err != nil {
			s.fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
		var upd database.InstanceUpdate
		if v, ok := fieldString(fields, "alias"); ok {
			v = strings.TrimSpace(v)
			upd.Alias = &v
		}
		if v, ok := fieldOptionalInt(fields, "employee_id"); ok {
			upd.SetEmployee, upd.EmployeeID = true, v
		}
		if v, ok := fieldBool(fields, "allow_send"); ok {
			upd.AllowSend = &v
		}
		if v, ok := fieldBool(fields, "corporate_asset_confirmed"); ok && v {
			upd.Confirm = &database.CorporateConfirmation{TermsVersion: currentTermsVersion, ConfirmedBy: actorOf(req)}
		}
		inst, err := s.messageStore.UpdateInstance(id, upd)
		if err != nil {
			s.serverError(w, "update instance", err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "instance": s.decorate(inst)})

	case http.MethodDelete:
		if s.instanceManager == nil {
			s.fail(w, http.StatusServiceUnavailable, "instance manager unavailable")
			return
		}
		if err := s.instanceManager.RemoveInstance(id); err != nil {
			if errors.Is(err, whatsapp.ErrInstanceNotFound) {
				s.fail(w, http.StatusNotFound, "instance not found")
				return
			}
			s.serverError(w, "remove instance", err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "instance removed; captured messages were kept"})

	default:
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleInstanceAction(w http.ResponseWriter, req *http.Request, id int, action string) {
	if s.instanceManager == nil {
		s.fail(w, http.StatusServiceUnavailable, "instance manager unavailable")
		return
	}

	switch action {
	case "qr":
		if req.Method != http.MethodGet {
			s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		status, qr, err := s.instanceManager.PairingQR(id)
		if errors.Is(err, whatsapp.ErrInstanceNotFound) {
			// Not pending any more: it either finished pairing or expired.
			inst, _ := s.messageStore.GetInstance(id)
			if inst == nil {
				s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "status": whatsapp.PairingExpired})
				return
			}
			s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "status": whatsapp.PairingPaired, "instance": s.decorate(inst)})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "status": status, "qr_code": qr})

	case "reconnect", "disconnect":
		if req.Method != http.MethodPost {
			s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var err error
		if action == "reconnect" {
			err = s.instanceManager.ReconnectInstance(id)
		} else {
			err = s.instanceManager.DisconnectInstance(id)
		}
		if err != nil {
			if errors.Is(err, whatsapp.ErrInstanceNotFound) {
				s.fail(w, http.StatusNotFound, "instance not found")
				return
			}
			s.serverError(w, action+" instance", err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "instance " + action + "ed"})

	default:
		s.fail(w, http.StatusNotFound, "unknown action")
	}
}

// ============================================================================
// AUDIT: FEED, VERSIONS, ACCESS LOG
// ============================================================================

func parseTimeParam(r *http.Request, key string) (*time.Time, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, false
	}
	return &t, true
}

// handleMessageFeed serves GET /api/messages/feed: captured messages across numbers, filtered by
// number, employee, department, chat, text, date range or deletion, newest first. Reading it is
// recorded in the access log.
func (s *Server) handleMessageFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	f := database.FeedFilter{
		ChatJID:     q.Get("chat_jid"),
		Query:       q.Get("q"),
		DeletedOnly: q.Get("deleted_only") == "true",
	}
	if ref := instanceRef(r); ref != "" {
		if s.instanceManager == nil {
			f.InstanceJID = ref
		} else if c, err := s.instanceManager.ResolveClient(ref); err == nil {
			f.InstanceJID = c.InstanceJID()
		} else {
			f.InstanceJID = ref
		}
	}
	if v, err := strconv.Atoi(q.Get("employee_id")); err == nil {
		f.EmployeeID = &v
	}
	if v, err := strconv.Atoi(q.Get("department_id")); err == nil {
		f.DepartmentID = &v
	}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil {
		f.Limit = v
	}
	var ok bool
	if f.Since, ok = parseTimeParam(r, "since"); !ok {
		s.fail(w, http.StatusBadRequest, "since must be RFC 3339")
		return
	}
	if f.Until, ok = parseTimeParam(r, "until"); !ok {
		s.fail(w, http.StatusBadRequest, "until must be RFC 3339")
		return
	}
	if f.Before, ok = parseTimeParam(r, "before"); !ok {
		s.fail(w, http.StatusBadRequest, "before must be RFC 3339")
		return
	}

	msgs, err := s.messageStore.ListMessageFeed(f)
	if err != nil {
		s.serverError(w, "list message feed", err)
		return
	}
	s.recordAccess(r, "feed", r.URL.RawQuery, len(msgs))
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "count": len(msgs), "messages": msgs})
}

// handleMessageVersions serves GET /api/messages/versions: the earlier texts of an edited or
// revoked message.
func (s *Server) handleMessageVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	instance, chat, id := q.Get("instance"), q.Get("chat_jid"), q.Get("message_id")
	if instance == "" || chat == "" || id == "" {
		s.fail(w, http.StatusBadRequest, "instance, chat_jid and message_id are required")
		return
	}
	if s.instanceManager != nil {
		if c, err := s.instanceManager.ResolveClient(instance); err == nil {
			instance = c.InstanceJID()
		}
	}
	versions, err := s.messageStore.GetMessageVersions(instance, chat, id)
	if err != nil {
		s.serverError(w, "get message versions", err)
		return
	}
	if versions == nil {
		versions = []types.MessageVersion{}
	}
	s.recordAccess(r, "versions", r.URL.RawQuery, len(versions))
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "versions": versions})
}

// recordAccess writes a read of message data to the access log. A failure is logged but never
// blocks the read.
func (s *Server) recordAccess(r *http.Request, action, params string, count int) {
	if len(params) > 500 {
		params = params[:500]
	}
	n := count
	err := s.messageStore.InsertAccessLog(types.AccessLogEntry{
		Actor:       actorOf(r),
		ClientID:    r.Header.Get("X-Actor"),
		Action:      action,
		Resource:    r.URL.Path,
		Params:      params,
		ResultCount: &n,
	})
	if err != nil {
		log.Printf("api: access log: %v", err)
	}
}

// handleAccessLog serves GET (recent entries) and POST (the MCP server records tool calls).
func (s *Server) handleAccessLog(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		entries, err := s.messageStore.ListAccessLog(limit)
		if err != nil {
			s.serverError(w, "list access log", err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "entries": entries})

	case http.MethodPost:
		var e types.AccessLogEntry
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&e); err != nil {
			s.fail(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if strings.TrimSpace(e.Action) == "" {
			s.fail(w, http.StatusBadRequest, "action is required")
			return
		}
		if e.Actor == "" {
			e.Actor = actorOf(r)
		}
		if len(e.Params) > 1000 {
			e.Params = e.Params[:1000]
		}
		if err := s.messageStore.InsertAccessLog(e); err != nil {
			s.serverError(w, "insert access log", err)
			return
		}
		s.writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true})

	default:
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ============================================================================
// PRIVACY (LGPD)
// ============================================================================

// handlePrivacyAnonymize serves POST /api/privacy/anonymize: removes one person's personal data
// from the captured history (LGPD art. 18) and deletes their stored media files.
func (s *Server) handlePrivacyAnonymize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Subject string `json:"subject"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Subject) == "" {
		s.fail(w, http.StatusBadRequest, "subject (phone number or JID) is required")
		return
	}

	res, err := s.messageStore.AnonymizeSubject(body.Subject, actorOf(r))
	if err != nil {
		s.serverError(w, "anonymize subject", err)
		return
	}

	filesRemoved := 0
	for _, ref := range res.Media {
		dir := filepath.Join("store", "media", sanitizePath(ref.ChatJID))
		matches, _ := filepath.Glob(filepath.Join(dir, sanitizePath(ref.MessageID)+"*"))
		for _, m := range matches {
			if os.Remove(m) == nil {
				filesRemoved++
			}
		}
	}
	for _, jid := range res.ChatJIDs {
		_ = os.Remove(filepath.Join("store", "media", sanitizePath(jid))) // only succeeds when empty
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"messages":      res.Messages,
		"chats":         len(res.ChatJIDs),
		"pseudonym":     res.Pseudonym,
		"media_removed": filesRemoved,
	})
}

// handlePrivacyPurge serves POST /api/privacy/purge: deletes messages older than the given number
// of days, the same operation the retention policy runs daily.
func (s *Server) handlePrivacyPurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Days < 1 {
		s.fail(w, http.StatusBadRequest, "days must be a positive number")
		return
	}
	removed, err := s.messageStore.PurgeOlderThan(time.Now().UTC().AddDate(0, 0, -body.Days), actorOf(r))
	if err != nil {
		s.serverError(w, "purge messages", err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "removed": removed})
}

// handlePrivacyLog serves GET /api/privacy/log.
func (s *Server) handlePrivacyLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := s.messageStore.ListPrivacyLog(limit)
	if err != nil {
		s.serverError(w, "list privacy log", err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "entries": entries})
}
