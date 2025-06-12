package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/gorilla/mux"
)

// Server represents a PHP server configuration
type Server struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Port      string `json:"port"`
	Directory string `json:"directory"`
	Running   bool   `json:"running"`
	VLAN      string `json:"vlan,omitempty"` // Add VLAN field
}

// AppConfig represents the application configuration that will be saved to disk
type AppConfig struct {
	Servers map[string]*Server `json:"servers"`
	NextID  int                `json:"nextID"`
}

// App struct
type App struct {
	ctx        context.Context
	servers    map[string]*Server
	nextID     int
	mu         sync.Mutex
	processes  map[string]*exec.Cmd
	configPath string
}

// NewApp creates a new App application struct
func NewApp() *App {
	// Get the user's home directory for storing config
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	// Create the config directory if it doesn't exist
	configDir := filepath.Join(homeDir, ".php-server-manager")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		os.MkdirAll(configDir, 0755)
	}

	configPath := filepath.Join(configDir, "config.json")

	return &App{
		servers:    make(map[string]*Server),
		nextID:     1,
		processes:  make(map[string]*exec.Cmd),
		configPath: configPath,
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Load saved configuration
	a.loadConfig()
}

// shutdown is called when the app is about to exit
func (a *App) shutdown(ctx context.Context) {
	// Stop all running servers
	for id, server := range a.servers {
		if server.Running {
			a.StopServer(id)
		}
	}

	// Save configuration before exit
	a.saveConfig()
}

// loadConfig loads the saved configuration from disk
func (a *App) loadConfig() {
	data, err := ioutil.ReadFile(a.configPath)
	if err != nil {
		// If the file doesn't exist, that's fine - we'll create it later
		return
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		return
	}

	a.servers = config.Servers
	a.nextID = config.NextID

	// Ensure all servers are marked as not running on startup
	for _, server := range a.servers {
		server.Running = false
	}
}

// saveConfig saves the current configuration to disk
func (a *App) saveConfig() {
	a.mu.Lock()
	defer a.mu.Unlock()

	config := AppConfig{
		Servers: a.servers,
		NextID:  a.nextID,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fmt.Printf("Error serializing configuration: %v\n", err)
		return
	}

	if err := ioutil.WriteFile(a.configPath, data, 0644); err != nil {
		fmt.Printf("Error saving configuration: %v\n", err)
	}
}

// GetServers returns all configured servers
func (a *App) GetServers() []*Server {
	a.mu.Lock()
	defer a.mu.Unlock()

	servers := make([]*Server, 0, len(a.servers))
	for _, server := range a.servers {
		servers = append(servers, server)
	}
	return servers
}

// CreateServer adds a new server configuration
func (a *App) CreateServer(name, port, directory string) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	id := strconv.Itoa(a.nextID)
	a.nextID++

	server := &Server{
		ID:        id,
		Name:      name,
		Port:      port,
		Directory: directory,
		Running:   false,
	}

	a.servers[id] = server

	// Save configuration after creating a server
	go a.saveConfig()

	return id
}

// CreateServerWithVLAN adds a new server configuration with VLAN
func (a *App) CreateServerWithVLAN(name, port, directory string, vlan string) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	id := strconv.Itoa(a.nextID)
	a.nextID++

	server := &Server{
		ID:        id,
		Name:      name,
		Port:      port,
		Directory: directory,
		Running:   false,
		VLAN:      vlan,
	}

	a.servers[id] = server

	// Save configuration after creating a server
	go a.saveConfig()

	return id
}

// UpdateServer updates an existing server configuration
func (a *App) UpdateServer(id, name, port, directory string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	server, exists := a.servers[id]
	if !exists {
		return false
	}

	// If the server is running, stop it first
	if server.Running {
		a.mu.Unlock()
		a.StopServer(id)
		a.mu.Lock()
	}

	server.Name = name
	server.Port = port
	server.Directory = directory

	// Save configuration after updating a server
	go a.saveConfig()

	return true
}

// DeleteServer removes a server configuration
func (a *App) DeleteServer(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	server, exists := a.servers[id]
	if !exists {
		return false
	}

	// If the server is running, stop it first
	if server.Running {
		a.mu.Unlock()
		a.StopServer(id)
		a.mu.Lock()
	}

	delete(a.servers, id)

	// Save configuration after deleting a server
	go a.saveConfig()

	return true
}

func getCurrentUsername() string {
	user, err := os.UserHomeDir()
	if err != nil {
		return "root"
	}
	return filepath.Base(user)
}

// StartServer starts a PHP server
func (a *App) StartServer(id string) bool {
	a.mu.Lock()
	server, exists := a.servers[id]
	if !exists || server.Running {
		a.mu.Unlock()
		return false
	}
	a.mu.Unlock()

	command := fmt.Sprintf("frankenphp php-server --listen 0.0.0.0:%s -r %s", server.Port, server.Directory)
	os.Setenv("PATH", "/usr/local/bin:"+os.Getenv("PATH"))
	username := getCurrentUsername()
	fullCommand := fmt.Sprintf("sudo -u %s /bin/bash -c '%s'", username, command)
	cmd := exec.Command("/bin/bash", "-c", fullCommand)

	// Set the working directory to the current directory
	cmd.Dir, _ = os.Getwd()

	// Start the command
	err := cmd.Start()
	if err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		return false
	}

	a.mu.Lock()
	a.processes[id] = cmd
	server.Running = true
	a.mu.Unlock()

	// Handle process completion
	go func() {
		cmd.Wait()
		a.mu.Lock()
		delete(a.processes, id)
		server.Running = false
		a.mu.Unlock()
	}()

	return true
}

// StopServer stops a running PHP server
func (a *App) StopServer(id string) bool {
	a.mu.Lock()
	server, exists := a.servers[id]
	if !exists || !server.Running {
		a.mu.Unlock()
		return false
	}

	cmd, exists := a.processes[id]
	if !exists {
		server.Running = false
		a.mu.Unlock()
		return true
	}
	a.mu.Unlock()

	// Kill the process
	if err := cmd.Process.Kill(); err != nil {
		fmt.Printf("Error stopping server: %v\n", err)
		return false
	}

	a.mu.Lock()
	delete(a.processes, id)
	server.Running = false
	a.mu.Unlock()

	return true
}

// GetServerStatus returns the status of a specific server
func (a *App) GetServerStatus(id string) (bool, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	server, exists := a.servers[id]
	if !exists {
		return false, false
	}
	
	return true, server.Running
}

// API handlers
func (a *App) handleGetServers(w http.ResponseWriter, r *http.Request) {
	servers := a.GetServers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

func (a *App) handleCreateServer(w http.ResponseWriter, r *http.Request) {
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

	id := a.CreateServer(serverData.Name, serverData.Port, serverData.Directory)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// handleCreateServerWithVLAN creates a new server with VLAN configuration
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

	// Get a free VLAN
	vlan, err := vlanManager.GetFreeVLAN()
	if err != nil {
		http.Error(w, "No available VLANs", http.StatusInternalServerError)
		return
	}

	id := a.CreateServerWithVLAN(serverData.Name, serverData.Port, serverData.Directory, vlan)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "vlan": vlan})
}

// handleDeleteServerWithVLAN deletes a server and releases its VLAN
func (a *App) handleDeleteServerWithVLAN(w http.ResponseWriter, r *http.Request, vlanManager *VLANManager) {
	vars := mux.Vars(r)
	id := vars["id"]

	a.mu.Lock()
	server, exists := a.servers[id]
	if !exists {
		a.mu.Unlock()
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}
	vlan := server.VLAN
	a.mu.Unlock()

	success := a.DeleteServer(id)
	if !success {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	// Release the VLAN
	if vlan != "" {
		vlanManager.ReleaseVLAN(vlan)
	}

	w.WriteHeader(http.StatusOK)
}

// handleStartServerWithVLAN starts a server and configures its VLAN
func (a *App) handleStartServerWithVLAN(w http.ResponseWriter, r *http.Request, vlanManager *VLANManager) {
	vars := mux.Vars(r)
	id := vars["id"]

	a.mu.Lock()
	server, exists := a.servers[id]
	if !exists {
		a.mu.Unlock()
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}
	vlan := server.VLAN
	a.mu.Unlock()

	success := a.StartServer(id)
	if !success {
		http.Error(w, "Failed to start server or server is already running", http.StatusBadRequest)
		return
	}

	// Configure the VLAN
	if vlan != "" {
		err := vlanManager.ConfigureVLAN(vlan)
		if err != nil {
			http.Error(w, "Failed to configure VLAN", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

// handleStopServerWithVLAN stops a server and removes its VLAN configuration
func (a *App) handleStopServerWithVLAN(w http.ResponseWriter, r *http.Request, vlanManager *VLANManager) {
	vars := mux.Vars(r)
	id := vars["id"]

	a.mu.Lock()
	server, exists := a.servers[id]
	if !exists {
		a.mu.Unlock()
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}
	vlan := server.VLAN
	a.mu.Unlock()

	success := a.StopServer(id)
	if !success {
		http.Error(w, "Failed to stop server or server is already stopped", http.StatusBadRequest)
		return
	}

	// Remove the VLAN configuration
	if vlan != "" {
		err := vlanManager.RemoveVLAN(vlan)
		if err != nil {
			http.Error(w, "Failed to remove VLAN configuration", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
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

func (a *App) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	success := a.DeleteServer(id)
	if !success {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleStartServer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	success := a.StartServer(id)
	if !success {
		http.Error(w, "Failed to start server or server is already running", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleStopServer(w http.ResponseWriter, r *http.Request) {
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

// Serve static files
func serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.ServeFile(w, r, "static/index.html")
		return
	}
	
	http.ServeFile(w, r, "static"+r.URL.Path)
}

// CORS middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// AuthMiddleware struct
type AuthMiddleware struct {
	password string
}

// NewAuthMiddleware creates a new AuthMiddleware
func NewAuthMiddleware(password string) *AuthMiddleware {
	return &AuthMiddleware{password: password}
}

// Middleware function for authentication
func (am *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for login and logout endpoints
		if r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}

		// Get the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if the Authorization header is valid
		if authHeader != "Bearer "+am.password {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

// HandleLogin handles the login request
func (am *AuthMiddleware) HandleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": am.password})
}

// HandleLogout handles the logout request
func (am *AuthMiddleware) HandleLogout(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// VLANManager struct
type VLANManager struct {
	subnet string
	usedVLANs map[string]bool
	mu sync.Mutex
}

// NewVLANManager creates a new VLANManager
func NewVLANManager(subnet string) *VLANManager {
	return &VLANManager{
		subnet: subnet,
		usedVLANs: make(map[string]bool),
	}
}

// GetFreeVLAN returns a free VLAN
func (vm *VLANManager) GetFreeVLAN() (string, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Find a free VLAN
	for i := 100; i < 200; i++ {
		vlan := fmt.Sprintf("vlan%d", i)
		if !vm.usedVLANs[vlan] {
			vm.usedVLANs[vlan] = true
			return vlan, nil
		}
	}

	return "", fmt.Errorf("no free VLANs available")
}

// ReleaseVLAN releases a VLAN
func (vm *VLANManager) ReleaseVLAN(vlan string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	delete(vm.usedVLANs, vlan)
}

// ConfigureVLAN configures a VLAN
func (vm *VLANManager) ConfigureVLAN(vlan string) error {
	// Placeholder for VLAN configuration logic
	fmt.Printf("Configuring VLAN %s\n", vlan)
	return nil
}

// RemoveVLAN removes a VLAN configuration
func (vm *VLANManager) RemoveVLAN(vlan string) error {
	// Placeholder for VLAN removal logic
	fmt.Printf("Removing VLAN %s\n", vlan)
	return nil
}

// handleGetInterfaces returns the list of VLAN interfaces
func (vm *VLANManager) handleGetInterfaces(w http.ResponseWriter, r *http.Request) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	interfaces := make([]string, 0, len(vm.usedVLANs))
	for vlan := range vm.usedVLANs {
		interfaces = append(interfaces, vlan)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(interfaces)
}

// handleGetStatus returns the status of the VLAN manager
func (vm *VLANManager) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	status := map[string]interface{}{
		"subnet": vm.subnet,
		"usedVLANs": vm.usedVLANs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func main() {
	// Initialize the App
	app := NewApp()
	app.startup(context.Background())
	defer app.shutdown(context.Background())

	// Initialize VLAN manager
	vlanManager := NewVLANManager("2a0e:b107:384:ee25::/64")

	// Create router
	r := mux.NewRouter()
	
	// Add authentication middleware
	authMiddleware := NewAuthMiddleware("admin123") // Default password, should be configurable
	
	// API endpoints with authentication
	api := r.PathPrefix("/api").Subrouter()
	api.Use(corsMiddleware)
	api.Use(authMiddleware.Middleware)
	api.HandleFunc("/servers", app.handleGetServers).Methods("GET")
	api.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		app.handleCreateServerWithVLAN(w, r, vlanManager)
	}).Methods("POST")
	api.HandleFunc("/servers/{id}", app.handleUpdateServer).Methods("PUT")
	api.HandleFunc("/servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		app.handleDeleteServerWithVLAN(w, r, vlanManager)
	}).Methods("DELETE")
	api.HandleFunc("/servers/{id}/start", func(w http.ResponseWriter, r *http.Request) {
		app.handleStartServerWithVLAN(w, r, vlanManager)
	}).Methods("POST")
	api.HandleFunc("/servers/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		app.handleStopServerWithVLAN(w, r, vlanManager)
	}).Methods("POST")
	api.HandleFunc("/servers/{id}/status", app.handleServerStatus).Methods("GET")
	
	// Authentication endpoints
	api.HandleFunc("/auth/login", authMiddleware.HandleLogin).Methods("POST")
	api.HandleFunc("/auth/logout", authMiddleware.HandleLogout).Methods("POST")
	
	// VLAN management endpoints
	api.HandleFunc("/vlan/interfaces", vlanManager.handleGetInterfaces).Methods("GET")
	api.HandleFunc("/vlan/status", vlanManager.handleGetStatus).Methods("GET")
	
	// Ensure the static directory exists
	os.MkdirAll("static", 0755)
	
	// Create index.html if it doesn't exist
	if _, err := os.Stat("static/index.html"); os.IsNotExist(err) {
		if err := createIndexHTML(); err != nil {
			log.Fatalf("Failed to create index.html: %v", err)
		}
	}
	
	// Static files
	r.PathPrefix("/").HandlerFunc(serveStatic)

	// Start web server on port 80
	port := ":80"
	fmt.Printf("PHP Server Manager is running at http://localhost%s\n", port)
	fmt.Println("Default password: admin123")
	log.Fatal(http.ListenAndServe(port, r))
}

// createIndexHTML creates the index.html file for the web UI
func createIndexHTML() error {
	content := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>PHP Server Manager</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 0;
            padding: 20px;
            line-height: 1.6;
        }
        h1, h2 {
            margin-bottom: 10px;
        }
        .container {
            max-width: 1000px;
            margin: 0 auto;
        }
        .server-list {
            margin-top: 20px;
            border: 1px solid #ddd;
            border-radius: 5px;
        }
        .server-item {
            padding: 10px;
            border-bottom: 1px solid #ddd;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .server-item:last-child {
            border-bottom: none;
        }
        .server-status {
            padding: 3px 8px;
            border-radius: 3px;
            font-size: 0.8em;
            font-weight: bold;
        }
        .status-running {
            background-color: #d4edda;
            color: #155724;
        }
        .status-stopped {
            background-color: #f8d7da;
            color: #721c24;
        }
        .btn-group {
            display: flex;
            gap: 10px;
        }
        button {
            padding: 5px 10px;
            border: none;
            border-radius: 3px;
            cursor: pointer;
        }
        .btn-primary {
            background-color: #007bff;
            color: white;
        }
        .btn-success {
            background-color: #28a745;
            color: white;
        }
        .btn-danger {
            background-color: #dc3545;
            color: white;
        }
        .btn-secondary {
            background-color: #6c757d;
            color: white;
        }
        .modal {
            display: none;
            position: fixed;
            z-index: 1;
            left: 0;
            top: 0;
            width: 100%;
            height: 100%;
            background-color: rgba(0,0,0,0.4);
        }
        .modal-content {
            background-color: #fefefe;
            margin: 15% auto;
            padding: 20px;
            border: 1px solid #888;
            width: 80%;
            max-width: 500px;
            border-radius: 5px;
        }
        .close {
            color: #aaa;
            float: right;
            font-size: 28px;
            font-weight: bold;
            cursor: pointer;
        }
        .close:hover {
            color: black;
        }
        .form-group {
            margin-bottom: 15px;
        }
        label {
            display: block;
            margin-bottom: 5px;
            font-weight: bold;
        }
        input[type="text"] {
            width: 100%;
            padding: 8px;
            border: 1px solid #ddd;
            border-radius: 3px;
            box-sizing: border-box;
        }
        .form-actions {
            display: flex;
            justify-content: flex-end;
            gap: 10px;
            margin-top: 20px;
        }
        .alert {
            padding: 10px;
            margin-bottom: 15px;
            border-radius: 3px;
        }
        .alert-success {
            background-color: #d4edda;
            color: #155724;
        }
        .alert-danger {
            background-color: #f8d7da;
            color: #721c24;
        }
        .hidden {
            display: none;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>PHP Server Manager</h1>
        <p>Manage your PHP development servers</p>
        
        <button id="add-server-btn" class="btn-primary">Add Server</button>
        
        <div id="alert" class="alert hidden"></div>
        
        <h2>Your Servers:</h2>
        <div id="server-list" class="server-list">
            <div id="loading">Loading servers...</div>
        </div>
    </div>
    
    <!-- Server Modal -->
    <div id="server-modal" class="modal">
        <div class="modal-content">
            <span class="close">&times;</span>
            <h2 id="modal-title">Server Configuration</h2>
            <form id="server-form">
                <input type="hidden" id="server-id">
                <div class="form-group">
                    <label for="server-name">Server Name:</label>
                    <input type="text" id="server-name" placeholder="My PHP Server" required>
                </div>
                <div class="form-group">
                    <label for="server-port">Port:</label>
                    <input type="text" id="server-port" placeholder="8000" required pattern="[0-9]+">
                </div>
                <div class="form-group">
                    <label for="server-directory">Document Root:</label>
                    <input type="text" id="server-directory" placeholder="/path/to/your/php/project" required>
                </div>
                <div class="form-actions">
                    <button type="button" id="cancel-server" class="btn-secondary">Cancel</button>
                    <button type="submit" id="save-server" class="btn-primary">Save</button>
                </div>
            </form>
        </div>
    </div>
    
    <!-- Confirmation Modal -->
    <div id="confirm-modal" class="modal">
        <div class="modal-content">
            <span class="close">&times;</span>
            <h2>Confirmation</h2>
            <p id="confirm-message">Are you sure you want to delete this server?</p>
            <div class="form-actions">
                <button type="button" id="cancel-confirm" class="btn-secondary">Cancel</button>
                <button type="button" id="confirm-action" class="btn-danger">Confirm</button>
            </div>
        </div>
    </div>
    <script>
        // DOM Elements
        const serverList = document.getElementById('server-list');
        const addServerBtn = document.getElementById('add-server-btn');
        const serverModal = document.getElementById('server-modal');
        const confirmModal = document.getElementById('confirm-modal');
        const serverForm = document.getElementById('server-form');
        const modalTitle = document.getElementById('modal-title');
        const serverIdInput = document.getElementById('server-id');
        const serverNameInput = document.getElementById('server-name');
        const serverPortInput = document.getElementById('server-port');
        const serverDirectoryInput = document.getElementById('server-directory');
        const alertElement = document.getElementById('alert');
        const confirmMessage = document.getElementById('confirm-message');
        const confirmAction = document.getElementById('confirm-action');
        // Modal close buttons
        document.querySelectorAll('.close, #cancel-server, #cancel-confirm').forEach(element => {
            element.addEventListener('click', () => {
                serverModal.style.display = 'none';
                confirmModal.style.display = 'none';
            });
        });
        // API Base URL
        const API_BASE = '/api';
        // Show alert message
        function showAlert(message, type) {
            alertElement.textContent = message;
            alertElement.className = 'alert alert-' + type;
            alertElement.classList.remove('hidden');
            setTimeout(() => {
                alertElement.classList.add('hidden');
            }, 3000);
        }
        // Load all servers
        async function loadServers() {
            try {
                const response = await fetch(API_BASE + '/servers');
                if (!response.ok) {
                    throw new Error('Failed to load servers');
                }
                
                const servers = await response.json();
                
                if (servers.length === 0) {
                    serverList.innerHTML = '<div class="server-item">No servers configured. Click "Add Server" to create one.</div>';
                    return;
                }
                
                serverList.innerHTML = '';
                servers.forEach(server => {
                    const statusClass = server.running ? 'status-running' : 'status-stopped';
                    const statusText = server.running ? 'Running' : 'Stopped';
                    
                    const serverItem = document.createElement('div');
                    serverItem.className = 'server-item';
                    serverItem.innerHTML = '<div>' +
                        '<strong>' + server.name + '</strong>' +
                        '<div>Port: ' + server.port + '</div>' +
                        '<div>Directory: ' + server.directory + '</div>' +
                        '<div>Status: <span class="server-status ' + statusClass + '">' + statusText + '</span></div>' +
                        '</div>' +
                        '<div class="btn-group">' +
                        (!server.running ? '<button class="btn-success start-server" data-id="' + server.id + '">Start</button>' : '') +
                        (server.running ? '<button class="btn-danger stop-server" data-id="' + server.id + '">Stop</button>' : '') +
                        '<button class="btn-secondary edit-server" data-id="' + server.id + 
                        '" data-name="' + server.name + 
                        '" data-port="' + server.port + 
                        '" data-directory="' + server.directory + '">Edit</button>' +
                        '<button class="btn-danger delete-server" data-id="' + server.id + '">Delete</button>' +
                        '</div>';
                    serverList.appendChild(serverItem);
                });
                
                // Add event listeners for server actions
                document.querySelectorAll('.start-server').forEach(btn => {
                    btn.addEventListener('click', startServer);
                });
                
                document.querySelectorAll('.stop-server').forEach(btn => {
                    btn.addEventListener('click', stopServer);
                });
                
                document.querySelectorAll('.edit-server').forEach(btn => {
                    btn.addEventListener('click', editServer);
                });
                
                document.querySelectorAll('.delete-server').forEach(btn => {
                    btn.addEventListener('click', showDeleteConfirmation);
                });
                
            } catch (error) {
                console.error('Error loading servers:', error);
                serverList.innerHTML = '<div class="server-item">Error loading servers. Please try again.</div>';
            }
        }
        // Show server modal for adding a server
        addServerBtn.addEventListener('click', () => {
            modalTitle.textContent = 'Add Server';
            serverIdInput.value = '';
            serverForm.reset();
            serverModal.style.display = 'block';
        });
        // Handle server form submission
        serverForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const id = serverIdInput.value;
            const name = serverNameInput.value;
            const port = serverPortInput.value;
            const directory = serverDirectoryInput.value;
            
            const serverData = {
                name,
                port,
                directory
            };
            
            try {
                let response;
                
                if (id) {
                    // Update existing server
                    response = await fetch(API_BASE + '/servers/' + id, {
                        method: 'PUT',
                        headers: {
                            'Content-Type': 'application/json'
                        },
                        body: JSON.stringify(serverData)
                    });
                    
                    if (!response.ok) {
                        throw new Error('Failed to update server');
                    }
                    
                    showAlert('Server updated successfully', 'success');
                } else {
                    // Create new server
                    response = await fetch(API_BASE + '/servers', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json'
                        },
                        body: JSON.stringify(serverData)
                    });
                    
                    if (!response.ok) {
                        throw new Error('Failed to create server');
                    }
                    
                    showAlert('Server created successfully', 'success');
                }
                
                serverModal.style.display = 'none';
                loadServers();
                
            } catch (error) {
                console.error('Error saving server:', error);
                showAlert(error.message, 'danger');
            }
        });
        // Edit server
        function editServer(e) {
            const button = e.target;
            const id = button.getAttribute('data-id');
            const name = button.getAttribute('data-name');
            const port = button.getAttribute('data-port');
            const directory = button.getAttribute('data-directory');
            
            modalTitle.textContent = 'Edit Server';
            serverIdInput.value = id;
            serverNameInput.value = name;
            serverPortInput.value = port;
            serverDirectoryInput.value = directory;
            
            serverModal.style.display = 'block';
        }
        // Show delete confirmation
        function showDeleteConfirmation(e) {
            const id = e.target.getAttribute('data-id');
            confirmMessage.textContent = 'Are you sure you want to delete this server?';
            confirmAction.setAttribute('data-id', id);
            confirmAction.setAttribute('data-action', 'delete');
            confirmModal.style.display = 'block';
        }
        // Handle confirmation action
        confirmAction.addEventListener('click', async () => {
            const id = confirmAction.getAttribute('data-id');
            const action = confirmAction.getAttribute('data-action');
            
            try {
                if (action === 'delete') {
                    const response = await fetch(API_BASE + '/servers/' + id, {
                        method: 'DELETE'
                    });
                    
                    if (!response.ok) {
                        throw new Error('Failed to delete server');
                    }
                    
                    showAlert('Server deleted successfully', 'success');
                }
                
                confirmModal.style.display = 'none';
                loadServers();
                
            } catch (error) {
                console.error('Error:', error);
                showAlert(error.message, 'danger');
            }
        });
        // Start server
        async function startServer(e) {
            const id = e.target.getAttribute('data-id');
            
            try {
                const response = await fetch(API_BASE + '/servers/' + id + '/start', {
                    method: 'POST'
                });
                
                if (!response.ok) {
                    throw new Error('Failed to start server');
                }
                
                showAlert('Server started successfully', 'success');
                loadServers();
                
            } catch (error) {
                console.error('Error starting server:', error);
                showAlert(error.message, 'danger');
            }
        }
        // Stop server
        async function stopServer(e) {
            const id = e.target.getAttribute('data-id');
            
            try {
                const response = await fetch(API_BASE + '/servers/' + id + '/stop', {
                    method: 'POST'
                });
                
                if (!response.ok) {
                    throw new Error('Failed to stop server');
                }
                
                showAlert('Server stopped successfully', 'success');
                loadServers();
                
            } catch (error) {
                console.error('Error stopping server:', error);
                showAlert(error.message, 'danger');
            }
        }
        
        // Load initial servers on page load
        window.addEventListener('load', loadServers);
    </script>
</body>
</html>`

	return ioutil.WriteFile("static/index.html", []byte(content), 0644)
}
