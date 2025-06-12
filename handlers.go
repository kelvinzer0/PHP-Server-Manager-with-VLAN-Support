package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// Enhanced handlers with VLAN support

func (a *App) handleGetServers(w http.ResponseWriter, r *http.Request) {
	servers := a.GetServers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

func (a *App) handleCreateServerWithVLAN(w http.ResponseWriter, r *http.Request, vlanManager *VLANManager) {
	var serverData struct {
		Name      string `json:"name"`
		Port      string `json:"port"`
		Directory string `json:"directory"`
	}

	if err := json.NewDecoder(r.Body).Decode(&serverData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate inputs
	if serverData.Name == "" || serverData.Port == "" || serverData.Directory == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	// Validate port is a number
	_, err := strconv.Atoi(serverData.Port)
	if err != nil {
		http.Error(w, "Port must be a number", http.StatusBadRequest)
		return
	}

	// Create VLAN interface for this port
	vlanInterface, err := vlanManager.CreateVLANInterface(serverData.Port)
	if err != nil {
		http.Error(w, "Failed to create VLAN interface: "+err.Error(), http.StatusInternalServerError)
		return
	}

	id := a.CreateServer(serverData.Name, serverData.Port, serverData.Directory)
	
	// Update server with VLAN information
	a.mu.Lock()
	if server, exists := a.servers[id]; exists {
		server.VLANInterface = vlanInterface.Name
		server.IPv6Address = vlanInterface.IPv6Address
	}
	a.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": id,
		"vlan_interface": vlanInterface.Name,
		"ipv6_address": vlanInterface.IPv6Address,
	})
}

func (a *App) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var serverData struct {
		Name      string `json:"name"`
		Port      string `json:"port"`
		Directory string `json:"directory"`
	}

	if err := json.NewDecoder(r.Body).Decode(&serverData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate inputs
	if serverData.Name == "" || serverData.Port == "" || serverData.Directory == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	// Validate port is a number
	_, err := strconv.Atoi(serverData.Port)
	if err != nil {
		http.Error(w, "Port must be a number", http.StatusBadRequest)
		return
	}

	success := a.UpdateServer(id, serverData.Name, serverData.Port, serverData.Directory)
	if !success {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleDeleteServerWithVLAN(w http.ResponseWriter, r *http.Request, vlanManager *VLANManager) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Get server info before deletion
	a.mu.Lock()
	server, exists := a.servers[id]
	var port string
	if exists {
		port = server.Port
	}
	a.mu.Unlock()

	success := a.DeleteServer(id)
	if !success {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	// Remove VLAN interface if server existed
	if port != "" {
		if err := vlanManager.RemoveVLANInterface(port); err != nil {
			// Log error but don't fail the deletion
			http.Error(w, "Server deleted but failed to remove VLAN interface: "+err.Error(), http.StatusPartialContent)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleStartServerWithVLAN(w http.ResponseWriter, r *http.Request, vlanManager *VLANManager) {
	vars := mux.Vars(r)
	id := vars["id"]

	success := a.StartServer(id)
	if !success {
		http.Error(w, "Failed to start server or server is already running", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleStopServerWithVLAN(w http.ResponseWriter, r *http.Request, vlanManager *VLANManager) {
	vars := mux.Vars(r)
	id := vars["id"]

	success := a.StopServer(id)
	if !success {
		http.Error(w, "Failed to stop server or server is already stopped", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	exists, running := a.GetServerStatus(id)
	if !exists {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"running": running})
}
