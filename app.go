package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	VLANInterface string `json:"vlan_interface,omitempty"`
	IPv6Address   string `json:"ipv6_address,omitempty"`
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
	a.saveConfig()
}

// loadConfig loads the saved configuration from disk
func (a *App) loadConfig() {
	data, err := ioutil.ReadFile(a.configPath)
	if err != nil {
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

	if server.Running {
		a.mu.Unlock()
		a.StopServer(id)
		a.mu.Lock()
	}

	server.Name = name
	server.Port = port
	server.Directory = directory

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

	if server.Running {
		a.mu.Unlock()
		a.StopServer(id)
		a.mu.Lock()
	}

	delete(a.servers, id)
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

	// Use IPv6 address if available, otherwise use 0.0.0.0
	listenAddr := "0.0.0.0"
	if server.IPv6Address != "" {
		listenAddr = "[" + server.IPv6Address + "]"
	}

	command := fmt.Sprintf("frankenphp php-server --listen %s:%s -r %s", listenAddr, server.Port, server.Directory)
	os.Setenv("PATH", "/usr/local/bin:"+os.Getenv("PATH"))
	username := getCurrentUsername()
	fullCommand := fmt.Sprintf("sudo -u %s /bin/bash -c '%s'", username, command)
	cmd := exec.Command("/bin/bash", "-c", fullCommand)

	cmd.Dir, _ = os.Getwd()

	err := cmd.Start()
	if err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		return false
	}

	a.mu.Lock()
	a.processes[id] = cmd
	server.Running = true
	a.mu.Unlock()

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
