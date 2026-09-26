package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"whatsapp-bridge/internal/types"
)

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// handleDepartments handles GET (list) and POST (create) for departments.
func (s *Server) handleDepartments(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		list, err := s.messageStore.ListDepartments()
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
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
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid request body"})
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "name is required"})
			return
		}

		dept, err := s.messageStore.CreateDepartment(body.Name, body.Description)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "department": dept})

	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
	}
}

// handleDepartmentByID handles GET, PUT, and DELETE for a specific department.
func (s *Server) handleDepartmentByID(w http.ResponseWriter, req *http.Request) {
	idStr := strings.TrimPrefix(req.URL.Path, "/api/departments/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid department id"})
		return
	}

	switch req.Method {
	case http.MethodGet:
		dept, err := s.messageStore.GetDepartment(id)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		if dept == nil {
			s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "error": "department not found"})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "department": dept})

	case http.MethodPut:
		var body struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid request body"})
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "name is required"})
			return
		}

		if err := s.messageStore.UpdateDepartment(id, body.Name, body.Description); err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "department updated"})

	case http.MethodDelete:
		if err := s.messageStore.DeleteDepartment(id); err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "department deleted"})

	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
	}
}

// handleEmployees handles GET (list/search) and POST (create) for employees.
func (s *Server) handleEmployees(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		q := req.URL.Query().Get("q")
		if q != "" {
			list, err := s.messageStore.FindEmployeesByName(q)
			if err != nil {
				s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
				return
			}
			if list == nil {
				list = []*types.Employee{}
			}
			s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "employees": list})
			return
		}

		var deptID *int
		if dStr := req.URL.Query().Get("department_id"); dStr != "" {
			if parsed, err := strconv.Atoi(dStr); err == nil {
				deptID = &parsed
			}
		}

		list, err := s.messageStore.ListEmployees(deptID)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
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
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid request body"})
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "name is required"})
			return
		}

		emp, err := s.messageStore.CreateEmployee(body.DepartmentID, body.Name, body.Role, body.Email)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "employee": emp})

	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
	}
}

// handleEmployeeByID handles GET, PUT, and DELETE for a specific employee.
func (s *Server) handleEmployeeByID(w http.ResponseWriter, req *http.Request) {
	idStr := strings.TrimPrefix(req.URL.Path, "/api/employees/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid employee id"})
		return
	}

	switch req.Method {
	case http.MethodGet:
		emp, err := s.messageStore.GetEmployee(id)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		if emp == nil {
			s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "error": "employee not found"})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "employee": emp})

	case http.MethodPut:
		var body struct {
			DepartmentID *int   `json:"department_id"`
			Name         string `json:"name"`
			Role         string `json:"role"`
			Email        string `json:"email"`
			Active       bool   `json:"active"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid request body"})
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "name is required"})
			return
		}

		if err := s.messageStore.UpdateEmployee(id, body.DepartmentID, body.Name, body.Role, body.Email, body.Active); err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "employee updated"})

	case http.MethodDelete:
		if err := s.messageStore.DeleteEmployee(id); err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "employee deleted"})

	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
	}
}

// handleInstances handles GET to list all instances and their statuses.
func (s *Server) handleInstances(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}

	instances, err := s.messageStore.ListInstances()
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if instances == nil {
		instances = []*types.Instance{}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "instances": instances})
}

// handleCreateInstancePair creates a new instance session and provides a pairing QR code.
func (s *Server) handleCreateInstancePair(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}

	if s.instanceManager == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "error": "multi-instance manager not initialized"})
		return
	}

	var body struct {
		Alias      string `json:"alias"`
		EmployeeID *int   `json:"employee_id"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)

	_, tempID, qrChan, err := s.instanceManager.CreateNewInstance(body.Alias, body.EmployeeID)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	// Wait up to 5 seconds for the first QR code from channel
	var qrCode string
	select {
	case item, ok := <-qrChan:
		if ok && item.Event == "code" {
			qrCode = item.Code
		}
	case <-time.After(5 * time.Second):
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"temp_id":  tempID,
		"qr_code":  qrCode,
		"message":  "Instance created. Scan the QR code to complete pairing.",
	})
}

// handleInstanceByID handles actions on a specific instance (GET, DELETE, reconnect, disconnect).
func (s *Server) handleInstanceByID(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/instances/")
	parts := strings.Split(path, "/")
	phoneJID := parts[0]

	if len(parts) > 1 {
		action := parts[1]
		if req.Method != http.MethodPost {
			s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
			return
		}

		if s.instanceManager == nil {
			s.writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"success": false, "error": "instance manager unavailable"})
			return
		}

		switch action {
		case "reconnect":
			if err := s.instanceManager.ReconnectInstance(phoneJID); err != nil {
				s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
				return
			}
			s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "instance reconnecting"})
			return

		case "disconnect":
			if err := s.instanceManager.DisconnectInstance(phoneJID); err != nil {
				s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
				return
			}
			s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "instance disconnected"})
			return

		default:
			s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "error": "unknown action"})
			return
		}
	}

	switch req.Method {
	case http.MethodGet:
		inst, err := s.messageStore.GetInstanceByPhone(phoneJID)
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		if inst == nil {
			s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "error": "instance not found"})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "instance": inst})

	case http.MethodDelete:
		if s.instanceManager != nil {
			_ = s.instanceManager.DeleteInstance(phoneJID)
		} else {
			_ = s.messageStore.DeleteInstance(phoneJID)
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "instance deleted"})

	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
	}
}
