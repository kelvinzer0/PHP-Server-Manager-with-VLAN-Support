package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// VLANManager manages VLAN interfaces and IPv6 addresses
type VLANManager struct {
	ipv6Prefix    string
	mu            sync.Mutex
	interfaces    map[string]*VLANInterface
	portToVLAN    map[string]string
}

// VLANInterface represents a VLAN interface configuration
type VLANInterface struct {
	Name        string `json:"name"`
	VLANID      int    `json:"vlan_id"`
	IPv6Address string `json:"ipv6_address"`
	Port        string `json:"port"`
	Active      bool   `json:"active"`
}

// NewVLANManager creates a new VLAN manager
func NewVLANManager(ipv6Prefix string) *VLANManager {
	return &VLANManager{
		ipv6Prefix: ipv6Prefix,
		interfaces: make(map[string]*VLANInterface),
		portToVLAN: make(map[string]string),
	}
}

// CreateVLANInterface creates a new VLAN interface for a given port
func (vm *VLANManager) CreateVLANInterface(port string) (*VLANInterface, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Check if VLAN already exists for this port
	if existingVLAN, exists := vm.portToVLAN[port]; exists {
		return vm.interfaces[existingVLAN], nil
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid port number: %s", port)
	}

	// Generate VLAN ID based on port (use port number as VLAN ID)
	vlanID := portNum
	interfaceName := fmt.Sprintf("vlan%d", vlanID)

	// Generate IPv6 address: prefix + ::port
	ipv6Addr := strings.Replace(vm.ipv6Prefix, "/64", "", 1) + "::" + port

	vlanInterface := &VLANInterface{
		Name:        interfaceName,
		VLANID:      vlanID,
		IPv6Address: ipv6Addr,
		Port:        port,
		Active:      false,
	}

	// Create the VLAN interface using ip command
	if err := vm.createLinuxVLANInterface(vlanInterface); err != nil {
		return nil, fmt.Errorf("failed to create VLAN interface: %v", err)
	}

	vm.interfaces[interfaceName] = vlanInterface
	vm.portToVLAN[port] = interfaceName

	return vlanInterface, nil
}

// createLinuxVLANInterface creates the actual VLAN interface on Linux
func (vm *VLANManager) createLinuxVLANInterface(vlan *VLANInterface) error {
	// Find the main network interface (usually wlan0 or similar)
	mainInterface, err := vm.getMainInterface()
	if err != nil {
		return fmt.Errorf("failed to get main interface: %v", err)
	}

	// Create VLAN interface
	cmd := exec.Command("sudo", "ip", "link", "add", "link", mainInterface, "name", vlan.Name, "type", "vlan", "id", strconv.Itoa(vlan.VLANID))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create VLAN interface: %v", err)
	}

	// Bring the interface up
	cmd = exec.Command("sudo", "ip", "link", "set", "dev", vlan.Name, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring up VLAN interface: %v", err)
	}

	// Add IPv6 address
	cmd = exec.Command("sudo", "ip", "-6", "addr", "add", vlan.IPv6Address+"/64", "dev", vlan.Name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add IPv6 address: %v", err)
	}

	vlan.Active = true
	return nil
}

// getMainInterface finds the main network interface
func (vm *VLANManager) getMainInterface() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			// Skip virtual interfaces
			if !strings.HasPrefix(iface.Name, "lo") && 
			   !strings.HasPrefix(iface.Name, "docker") && 
			   !strings.HasPrefix(iface.Name, "veth") &&
			   !strings.HasPrefix(iface.Name, "br-") {
				return iface.Name, nil
			}
		}
	}

	return "wlan0", nil // Default fallback
}

// RemoveVLANInterface removes a VLAN interface
func (vm *VLANManager) RemoveVLANInterface(port string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	vlanName, exists := vm.portToVLAN[port]
	if !exists {
		return nil // Already removed or never existed
	}

	vlan := vm.interfaces[vlanName]

	// Remove the VLAN interface
	cmd := exec.Command("sudo", "ip", "link", "delete", vlan.Name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove VLAN interface: %v", err)
	}

	delete(vm.interfaces, vlanName)
	delete(vm.portToVLAN, port)

	return nil
}

// GetVLANForPort returns the VLAN interface for a given port
func (vm *VLANManager) GetVLANForPort(port string) *VLANInterface {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vlanName, exists := vm.portToVLAN[port]; exists {
		return vm.interfaces[vlanName]
	}
	return nil
}

// HTTP handlers for VLAN management
func (vm *VLANManager) handleGetInterfaces(w http.ResponseWriter, r *http.Request) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	interfaces := make([]*VLANInterface, 0, len(vm.interfaces))
	for _, iface := range vm.interfaces {
		interfaces = append(interfaces, iface)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(interfaces)
}

func (vm *VLANManager) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	status := map[string]interface{}{
		"ipv6_prefix":     vm.ipv6Prefix,
		"active_vlans":    len(vm.interfaces),
		"port_mappings":   vm.portToVLAN,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
