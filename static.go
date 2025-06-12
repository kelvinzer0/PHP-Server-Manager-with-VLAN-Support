package main

import (
	"io/ioutil"
	"net/http"
)

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

// createIndexHTML creates the index.html file for the web UI with authentication
func createIndexHTML() error {
	content := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>PHP Server Manager with VLAN</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 0;
            padding: 20px;
            line-height: 1.6;
        }
        .login-container {
            max-width: 400px;
            margin: 100px auto;
            padding: 20px;
            border: 1px solid #ddd;
            border-radius: 5px;
            background-color: #f9f9f9;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
        }
        .vlan-info {
            background-color: #e7f3ff;
            padding: 10px;
            border-radius: 5px;
            margin-bottom: 20px;
        }
        .server-list {
            margin-top: 20px;
            border: 1px solid #ddd;
            border-radius: 5px;
        }
        .server-item {
            padding: 15px;
            border-bottom: 1px solid #ddd;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .server-item:last-child {
            border-bottom: none;
        }
        .server-details {
            flex-grow: 1;
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
            padding: 8px 15px;
            border: none;
            border-radius: 3px;
            cursor: pointer;
            font-size: 14px;
        }
        .btn-primary { background-color: #007bff; color: white; }
        .btn-success { background-color: #28a745; color: white; }
        .btn-danger { background-color: #dc3545; color: white; }
        .btn-secondary { background-color: #6c757d; color: white; }
        .btn-warning { background-color: #ffc107; color: black; }
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
            margin: 10% auto;
            padding: 20px;
            border: 1px solid #888;
            width: 80%;
            max-width: 600px;
            border-radius: 5px;
        }
        .close {
            color: #aaa;
            float: right;
            font-size: 28px;
            font-weight: bold;
            cursor: pointer;
        }
        .close:hover { color: black; }
        .form-group {
            margin-bottom: 15px;
        }
        label {
            display: block;
            margin-bottom: 5px;
            font-weight: bold;
        }
        input[type="text"], input[type="password"] {
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
        .alert-success { background-color: #d4edda; color: #155724; }
        .alert-danger { background-color: #f8d7da; color: #721c24; }
        .alert-info { background-color: #d1ecf1; color: #0c5460; }
        .hidden { display: none; }
        .vlan-status {
            font-size: 0.9em;
            color: #666;
        }
    </style>
</head>
<body>
    <!-- Login Form -->
    <div id="login-container" class="login-container">
        <h2>PHP Server Manager Login</h2>
        <form id="login-form">
            <div class="form-group">
                <label for="password">Password:</label>
                <input type="password" id="password" required>
            </div>
            <div class="form-actions">
                <button type="submit" class="btn-primary">Login</button>
            </div>
        </form>
        <div id="login-alert" class="alert hidden"></div>
    </div>

    <!-- Main Application -->
    <div id="main-app" class="container hidden">
        <div class="header">
            <h1>PHP Server Manager with VLAN</h1>
            <div>
                <button id="vlan-status-btn" class="btn-warning">VLAN Status</button>
                <button id="logout-btn" class="btn-secondary">Logout</button>
            </div>
        </div>
        
        <div class="vlan-info">
            <strong>IPv6 VLAN Configuration:</strong> 2a0e:b107:384:ee25::/64<br>
            <span class="vlan-status">Each server gets a unique IPv6 address based on its port number</span>
        </div>
        
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
                    <small>Note: A VLAN interface will be created with IPv6 address 2a0e:b107:384:ee25::PORT</small>
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
    
    <!-- VLAN Status Modal -->
    <div id="vlan-modal" class="modal">
        <div class="modal-content">
            <span class="close">&times;</span>
            <h2>VLAN Status</h2>
            <div id="vlan-content">Loading VLAN information...</div>
        </div>
    </div>

    <script>
        // Authentication token
        let authToken = localStorage.getItem('authToken');
        
        // DOM Elements
        const loginContainer = document.getElementById('login-container');
        const mainApp = document.getElementById('main-app');
        const loginForm = document.getElementById('login-form');
        const loginAlert = document.getElementById('login-alert');
        const logoutBtn = document.getElementById('logout-btn');
        const vlanStatusBtn = document.getElementById('vlan-status-btn');
        const vlanModal = document.getElementById('vlan-modal');
        const vlanContent = document.getElementById('vlan-content');
        
        // Check if user is already logged in
        if (authToken) {
            showMainApp();
        }
        
        // Login form handler
        loginForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const password = document.getElementById('password').value;
            
            try {
                const response = await fetch('/api/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ password })
                });
                
                if (response.ok) {
                    const data = await response.json();
                    authToken = data.token;
                    localStorage.setItem('authToken', authToken);
                    showMainApp();
                } else {
                    showLoginAlert('Invalid password', 'danger');
                }
            } catch (error) {
                showLoginAlert('Login failed: ' + error.message, 'danger');
            }
        });
        
        // Logout handler
        logoutBtn.addEventListener('click', async () => {
            try {
                await fetch('/api/auth/logout', {
                    method: 'POST',
                    headers: { 'Authorization': 'Bearer ' + authToken }
                });
            } catch (error) {
                console.error('Logout error:', error);
            }
            
            authToken = null;
            localStorage.removeItem('authToken');
            showLoginForm();
        });
        
        // VLAN status handler
        vlanStatusBtn.addEventListener('click', async () => {
            try {
                const response = await fetch('/api/vlan/status', {
                    headers: { 'Authorization': 'Bearer ' + authToken }
                });
                
                if (response.ok) {
                    const data = await response.json();
                    vlanContent.innerHTML = '<pre>' + JSON.stringify(data, null, 2) + '</pre>';
                    vlanModal.style.display = 'block';
                } else {
                    showAlert('Failed to load VLAN status', 'danger');
                }
            } catch (error) {
                showAlert('Error loading VLAN status: ' + error.message, 'danger');
            }
        });
        
        function showLoginForm() {
            loginContainer.classList.remove('hidden');
            mainApp.classList.add('hidden');
        }
        
        function showMainApp() {
            loginContainer.classList.add('hidden');
            mainApp.classList.remove('hidden');
            loadServers();
        }
        
        function showLoginAlert(message, type) {
            loginAlert.textContent = message;
            loginAlert.className = 'alert alert-' + type;
            loginAlert.classList.remove('hidden');
            setTimeout(() => loginAlert.classList.add('hidden'), 3000);
        }
        
        // Rest of the JavaScript code for server management...
        // (Similar to original but with authentication headers)
        
        const serverList = document.getElementById('server-list');
        const addServerBtn = document.getElementById('add-server-btn');
        const serverModal = document.getElementById('server-modal');
        const serverForm = document.getElementById('server-form');
        const modalTitle = document.getElementById('modal-title');
        const serverIdInput = document.getElementById('server-id');
        const serverNameInput = document.getElementById('server-name');
        const serverPortInput = document.getElementById('server-port');
        const serverDirectoryInput = document.getElementById('server-directory');
        const alertElement = document.getElementById('alert');
        
        // Modal close handlers
        document.querySelectorAll('.close, #cancel-server').forEach(element => {
            element.addEventListener('click', () => {
                serverModal.style.display = 'none';
                vlanModal.style.display = 'none';
            });
        });
        
        function showAlert(message, type) {
            alertElement.textContent = message;
            alertElement.className = 'alert alert-' + type;
            alertElement.classList.remove('hidden');
            setTimeout(() => alertElement.classList.add('hidden'), 3000);
        }
        
        async function loadServers() {
            try {
                const response = await fetch('/api/servers', {
                    headers: { 'Authorization': 'Bearer ' + authToken }
                });
                
                if (!response.ok) {
                    if (response.status === 401) {
                        showLoginForm();
                        return;
                    }
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
                    
                    const vlanInfo = server.vlan_interface ? 
                        '<div class="vlan-status">VLAN: ' + server.vlan_interface + ' | IPv6: ' + server.ipv6_address + '</div>' : 
                        '<div class="vlan-status">No VLAN configured</div>';
                    
                    const serverItem = document.createElement('div');
                    serverItem.className = 'server-item';
                    serverItem.innerHTML = '<div class="server-details">' +
                        '<strong>' + server.name + '</strong>' +
                        '<div>Port: ' + server.port + '</div>' +
                        '<div>Directory: ' + server.directory + '</div>' +
                        vlanInfo +
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
                
                // Add event listeners
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
                    btn.addEventListener('click', deleteServer);
                });
                
            } catch (error) {
                console.error('Error loading servers:', error);
                serverList.innerHTML = '<div class="server-item">Error loading servers. Please try again.</div>';
            }
        }
        
        // Server management functions with authentication
        addServerBtn.addEventListener('click', () => {
            modalTitle.textContent = 'Add Server';
            serverIdInput.value = '';
            serverForm.reset();
            serverModal.style.display = 'block';
        });
        
        serverForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const id = serverIdInput.value;
            const name = serverNameInput.value;
            const port = serverPortInput.value;
            const directory = serverDirectoryInput.value;
            
            const serverData = { name, port, directory };
            
            try {
                let response;
                
                if (id) {
                    response = await fetch('/api/servers/' + id, {
                        method: 'PUT',
                        headers: {
                            'Content-Type': 'application/json',
                            'Authorization': 'Bearer ' + authToken
                        },
                        body: JSON.stringify(serverData)
                    });
                    showAlert('Server updated successfully', 'success');
                } else {
                    response = await fetch('/api/servers', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                            'Authorization': 'Bearer ' + authToken
                        },
                        body: JSON.stringify(serverData)
                    });
                    showAlert('Server created successfully with VLAN interface', 'success');
                }
                
                if (!response.ok) {
                    throw new Error('Failed to save server');
                }
                
                serverModal.style.display = 'none';
                loadServers();
                
            } catch (error) {
                console.error('Error saving server:', error);
                showAlert(error.message, 'danger');
            }
        });
        
        function editServer(e) {
            const button = e.target;
            modalTitle.textContent = 'Edit Server';
            serverIdInput.value = button.getAttribute('data-id');
            serverNameInput.value = button.getAttribute('data-name');
            serverPortInput.value = button.getAttribute('data-port');
            serverDirectoryInput.value = button.getAttribute('data-directory');
            serverModal.style.display = 'block';
        }
        
        async function deleteServer(e) {
            if (!confirm('Are you sure you want to delete this server and its VLAN interface?')) return;
            
            const id = e.target.getAttribute('data-id');
            
            try {
                const response = await fetch('/api/servers/' + id, {
                    method: 'DELETE',
                    headers: { 'Authorization': 'Bearer ' + authToken }
                });
                
                if (!response.ok) {
                    throw new Error('Failed to delete server');
                }
                
                showAlert('Server and VLAN interface deleted successfully', 'success');
                loadServers();
                
            } catch (error) {
                console.error('Error deleting server:', error);
                showAlert(error.message, 'danger');
            }
        }
        
        async function startServer(e) {
            const id = e.target.getAttribute('data-id');
            
            try {
                const response = await fetch('/api/servers/' + id + '/start', {
                    method: 'POST',
                    headers: { 'Authorization': 'Bearer ' + authToken }
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
        
        async function stopServer(e) {
            const id = e.target.getAttribute('data-id');
            
            try {
                const response = await fetch('/api/servers/' + id + '/stop', {
                    method: 'POST',
                    headers: { 'Authorization': 'Bearer ' + authToken }
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
    </script>
</body>
</html>`

	return ioutil.WriteFile("static/index.html", []byte(content), 0644)
}
