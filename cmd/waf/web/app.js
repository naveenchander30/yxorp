// Global State
let currentConfig = null;
let trafficChart = null;
let lastTotalRequests = 0;
let allLogs = [];
let currentFilter = 'all';
let isConnected = false;
let pollInterval = null;

// Initialization
document.addEventListener('DOMContentLoaded', () => {
    initCharts();
    loadConfig();
    startPolling();
    switchTab('dashboard');
    setupKeyboardShortcuts();
});

// Keyboard Shortcuts
function setupKeyboardShortcuts() {
    document.addEventListener('keydown', (e) => {
        if (e.key === 'r' || e.key === 'R') {
            if (!e.target.matches('input, textarea')) {
                e.preventDefault();
                fetchData();
            }
        }
    });
}

// Navigation
function switchTab(tabId) {
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
    const navItem = document.querySelector(`.nav-item[data-tab="${tabId}"]`);
    if (navItem) navItem.classList.add('active');

    document.querySelectorAll('.view').forEach(el => el.classList.remove('active'));
    document.getElementById(`view-${tabId}`).classList.add('active');

    const titles = {
        'dashboard': 'Dashboard',
        'backends': 'Backend Servers',
        'rules': 'Security Rules',
        'settings': 'Configuration'
    };
    document.getElementById('page-title').innerText = titles[tabId] || 'Dashboard';

    if (tabId === 'rules') loadRules();
    if (tabId === 'settings') loadConfig();
    if (tabId === 'backends') loadBackends();
}

// Charts
function initCharts() {
    const ctx = document.getElementById('trafficChart').getContext('2d');
    const gradient = ctx.createLinearGradient(0, 0, 0, 300);
    gradient.addColorStop(0, 'rgba(59, 130, 246, 0.4)');
    gradient.addColorStop(1, 'rgba(59, 130, 246, 0)');

    trafficChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: Array(30).fill(''),
            datasets: [{
                label: 'Requests/sec',
                data: Array(30).fill(0),
                borderColor: '#3b82f6',
                backgroundColor: gradient,
                borderWidth: 2,
                pointRadius: 0,
                fill: true,
                tension: 0.4
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: false },
                tooltip: {
                    backgroundColor: '#09090b',
                    borderColor: '#27272a',
                    borderWidth: 1,
                    titleColor: '#f4f4f5',
                    bodyColor: '#f4f4f5',
                    padding: 12,
                    displayColors: false,
                    callbacks: {
                        label: (context) => `${context.parsed.y} req/s`
                    }
                }
            },
            scales: {
                x: { display: false },
                y: {
                    beginAtZero: true,
                    grid: {
                        color: '#27272a',
                        drawBorder: false
                    },
                    ticks: {
                        color: '#71717a',
                        font: { family: 'JetBrains Mono' }
                    }
                }
            },
            animation: { duration: 300 }
        }
    });
}

// Data Polling
function startPolling() {
    fetchData();
    pollInterval = setInterval(fetchData, 2000);
}

function stopPolling() {
    if (pollInterval) {
        clearInterval(pollInterval);
        pollInterval = null;
    }
}

async function fetchData() {
    try {
        const [vars, stats, logs, degradation] = await Promise.all([
            fetch('/debug/vars').then(r => r.json()),
            fetch('/api/stats').then(r => r.json()),
            fetch('/api/logs').then(r => r.json()),
            fetch('/api/degradation').then(r => r.json()).catch(() => ({ mode: 0, components: {} }))
        ]);

        updateDashboard(vars, stats, logs, degradation);
        updateConnectionStatus(true);
    } catch (e) {
        console.error('Poll Error:', e);
        updateConnectionStatus(false);
    }
}

function updateConnectionStatus(connected) {
    isConnected = connected;
    const statusDot = document.getElementById('status-dot');
    const statusText = document.getElementById('connection-status');
    
    if (connected) {
        statusDot.className = 'status-dot connected';
        statusText.innerText = 'Connected';
        statusText.style.color = '#10b981';
    } else {
        statusDot.className = 'status-dot disconnected';
        statusText.innerText = 'Disconnected';
        statusText.style.color = '#ef4444';
    }
}

function updateDashboard(vars, stats, logs, degradation) {
    const total = vars.requests_total || 0;
    const blocked = vars.requests_blocked || 0;
    const latencyTotal = vars.latency_total_ms || 0;

    // RPS Calculation with animation
    const delta = total - lastTotalRequests;
    if (lastTotalRequests !== 0) {
        const rps = Math.round(delta / 2);
        animateNumber('rps', rps);

        // Update Chart
        trafficChart.data.datasets[0].data.push(rps);
        trafficChart.data.datasets[0].data.shift();
        trafficChart.update('none');
    }
    lastTotalRequests = total;

    animateNumber('blocked-count', blocked);
    
    const avgLatency = total > 0 ? (latencyTotal / total).toFixed(1) : '0';
    document.getElementById('latency').innerText = avgLatency;
    document.getElementById('latency-unit').style.display = 'inline';

    // Uptime
    document.getElementById('uptime').innerText = stats.uptime || '0s';

    // System Health
    updateSystemHealth(degradation);

    // Logs
    allLogs = logs || [];
    renderLogs();
}

function animateNumber(elementId, targetValue) {
    const element = document.getElementById(elementId);
    const currentValue = parseInt(element.innerText) || 0;
    
    if (currentValue === targetValue) return;
    
    const duration = 500;
    const steps = 20;
    const stepValue = (targetValue - currentValue) / steps;
    const stepDuration = duration / steps;
    
    let currentStep = 0;
    const timer = setInterval(() => {
        currentStep++;
        const newValue = Math.round(currentValue + (stepValue * currentStep));
        element.innerText = newValue;
        
        if (currentStep >= steps) {
            element.innerText = targetValue;
            clearInterval(timer);
        }
    }, stepDuration);
}

function updateSystemHealth(degradation) {
    const modeNames = ['Normal', 'Partial Degradation', 'Full Degradation'];
    const modeClasses = ['badge-success', 'badge-warning', 'badge-danger'];
    
    const badge = document.getElementById('degradation-badge');
    badge.innerText = modeNames[degradation.mode] || 'Normal';
    badge.className = `badge ${modeClasses[degradation.mode] || 'badge-success'}`;

    const healthGrid = document.getElementById('health-grid');
    const components = degradation.components || {};
    
    const statusNames = ['Healthy', 'Degraded', 'Failed'];
    const statusClasses = ['healthy', 'degraded', 'failed'];
    
    const componentNames = {
        'rule_engine': 'Rules',
        'rate_limiter': 'Rate Limit',
        'circuit_breaker': 'Circuit Breaker',
        'health_check': 'Health Check',
        'metrics': 'Metrics',
        'logging': 'Logging'
    };

    healthGrid.innerHTML = '';
    for (const [key, status] of Object.entries(components)) {
        const div = document.createElement('div');
        div.className = 'health-item';
        div.innerHTML = `
            <div class="health-item-name">${componentNames[key] || key}</div>
            <div class="health-status">
                <div class="health-dot ${statusClasses[status]}"></div>
                ${statusNames[status]}
            </div>
        `;
        healthGrid.appendChild(div);
    }
}

function renderLogs() {
    const tbody = document.querySelector('#logs-table tbody');
    tbody.innerHTML = '';
    
    let filteredLogs = allLogs;
    if (currentFilter === 'blocked') {
        filteredLogs = allLogs.filter(log => log.action === 'BLOCKED');
    } else if (currentFilter === 'allowed') {
        filteredLogs = allLogs.filter(log => log.action === 'ALLOWED');
    }

    if (filteredLogs.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--text-muted);padding:2rem;">No events to display</td></tr>';
        return;
    }

    filteredLogs.slice(0, 50).forEach(log => {
        const tr = document.createElement('tr');

        let statusColor = 'text-success';
        if (log.status_code >= 400 && log.status_code < 500) statusColor = 'text-warning';
        if (log.status_code >= 500) statusColor = 'text-danger';

        const actionBadge = log.action === 'BLOCKED' 
            ? '<span class="badge badge-danger">Blocked</span>' 
            : '<span class="badge badge-success">Allowed</span>';

        const time = new Date(log.timestamp).toLocaleTimeString();
        
        tr.innerHTML = `
            <td class="font-mono text-muted">${time}</td>
            <td class="font-bold">${log.method}</td>
            <td class="font-mono text-primary" style="max-width:200px;overflow:hidden;text-overflow:ellipsis;" title="${log.path}">${log.path}</td>
            <td class="font-mono">${log.client_ip}</td>
            <td class="${statusColor} font-bold">${log.status_code}</td>
            <td class="font-mono text-muted">${log.latency}</td>
            <td>${actionBadge}</td>
        `;
        tbody.appendChild(tr);
    });
}

function filterLogs() {
    currentFilter = document.getElementById('log-filter').value;
    renderLogs();
}

// Backends
async function loadBackends() {
    const grid = document.getElementById('backends-grid');
    grid.innerHTML = '<div class="card"><div class="skeleton skeleton-card"></div></div>';

    try {
        const backends = await fetch('/api/backends').then(r => r.json());
        
        if (!backends || backends.length === 0) {
            grid.innerHTML = '<div class="card card-full"><p style="text-align:center;color:var(--text-muted);padding:2rem;">No backends configured</p></div>';
            return;
        }

        grid.innerHTML = '';
        backends.forEach(backend => {
            const card = document.createElement('div');
            card.className = `card backend-card ${backend.alive ? 'online' : 'offline'}`;
            
            const cb = backend.circuit_breaker;
            const cbStateClass = cb.state === 'closed' ? 'closed' : (cb.state === 'open' ? 'open' : 'half-open');

            card.innerHTML = `
                <div class="backend-header">
                    <div class="backend-url">${backend.url}</div>
                    <span class="backend-status ${backend.alive ? 'online' : 'offline'}">
                        ${backend.alive ? 'Online' : 'Offline'}
                    </span>
                </div>
                <div class="backend-metrics">
                    <div class="metric-item">
                        <div class="metric-label">Total Requests</div>
                        <div class="metric-value">${cb.total_requests || 0}</div>
                    </div>
                    <div class="metric-item">
                        <div class="metric-label">Successes</div>
                        <div class="metric-value text-success">${cb.successes || 0}</div>
                    </div>
                    <div class="metric-item">
                        <div class="metric-label">Failures</div>
                        <div class="metric-value text-danger">${cb.failures || 0}</div>
                    </div>
                    <div class="metric-item">
                        <div class="metric-label">Rejected</div>
                        <div class="metric-value text-warning">${cb.total_rejected || 0}</div>
                    </div>
                </div>
                <div class="circuit-breaker-indicator">
                    <div class="cb-header">
                        <span style="font-size:0.75rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.05em;">Circuit Breaker</span>
                        <span class="cb-state ${cbStateClass}">${cb.state}</span>
                    </div>
                    <div class="cb-stats">
                        <span>Threshold: ${cb.threshold}</span>
                    </div>
                </div>
            `;
            grid.appendChild(card);
        });
    } catch (e) {
        console.error('Failed to load backends:', e);
        showToast('Failed to load backends', 'error');
        grid.innerHTML = '<div class="card card-full"><p style="text-align:center;color:var(--text-danger);padding:2rem;">Error loading backends</p></div>';
    }
}

// Configuration
async function loadConfig() {
    try {
        const res = await fetch('/api/config');
        const cfg = await res.json();
        currentConfig = cfg;

        document.getElementById('cfg-port').value = cfg.server.port;
        document.getElementById('cfg-targets').value = cfg.proxy.targets.join(', ');
        document.getElementById('cfg-ratelimit').value = cfg.security.rate_limit.requests_per_minute;
        document.getElementById('cfg-maxbody').value = cfg.security.max_body_size || 10485760;
    } catch (e) {
        console.error('Failed to load config', e);
        showToast('Failed to load configuration', 'error');
    }
}

async function saveConfig(e) {
    e.preventDefault();
    if (!currentConfig) {
        showToast('No configuration loaded', 'error');
        return;
    }

    const newCfg = JSON.parse(JSON.stringify(currentConfig));
    newCfg.server.port = document.getElementById('cfg-port').value;

    const targetsStr = document.getElementById('cfg-targets').value;
    newCfg.proxy.targets = targetsStr.split(',').map(s => s.trim()).filter(s => s);

    newCfg.security.rate_limit.requests_per_minute = parseInt(document.getElementById('cfg-ratelimit').value);
    newCfg.security.max_body_size = parseInt(document.getElementById('cfg-maxbody').value);

    try {
        const res = await fetch('/api/config', {
            method: 'POST',
            body: JSON.stringify(newCfg),
            headers: { 'Content-Type': 'application/json' }
        });
        
        if (res.ok) {
            showToast('Configuration saved successfully', 'success');
            currentConfig = newCfg;
        } else {
            const error = await res.text();
            showToast(`Failed to save: ${error}`, 'error');
        }
    } catch (err) {
        showToast(`Failed to save: ${err.message}`, 'error');
    }
}

async function loadRules() {
    const container = document.getElementById('rules-container');
    container.innerHTML = '<div class="card"><div class="skeleton skeleton-card"></div></div>';

    try {
        const res = await fetch('/api/config');
        const cfg = await res.json();
        const rules = cfg.security.rules || [];

        if (rules.length === 0) {
            container.innerHTML = '<div class="card card-full"><p style="text-align:center;color:var(--text-muted);padding:2rem;">No security rules configured</p></div>';
            return;
        }

        container.innerHTML = '';
        rules.forEach((rule) => {
            const el = document.createElement('div');
            el.className = 'rule-card';
            el.innerHTML = `
                <div class="rule-header">
                    <span class="rule-name">${rule.name}</span>
                    <span class="badge badge-success">Active</span>
                </div>
                <div class="rule-pattern">${rule.pattern}</div>
                <div class="rule-meta">
                    <span>Target: <strong>${rule.location}</strong></span>
                </div>
            `;
            container.appendChild(el);
        });
    } catch (e) {
        console.error('Load rules failed', e);
        showToast('Failed to load rules', 'error');
    }
}

// Export Logs
function exportLogs() {
    if (allLogs.length === 0) {
        showToast('No logs to export', 'info');
        return;
    }

    const dataStr = JSON.stringify(allLogs, null, 2);
    const blob = new Blob([dataStr], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    
    const a = document.createElement('a');
    a.href = url;
    a.download = `yxorp-logs-${Date.now()}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    
    showToast('Logs exported successfully', 'success');
}

// Toast Notifications
function showToast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    
    const icon = type === 'success' ? '✓' : (type === 'error' ? '✕' : 'ℹ');
    toast.innerHTML = `
        <span style="font-size:1.25rem;">${icon}</span>
        <span>${message}</span>
    `;
    
    container.appendChild(toast);
    
    setTimeout(() => {
        toast.style.opacity = '0';
        toast.style.transform = 'translateX(400px)';
        setTimeout(() => container.removeChild(toast), 300);
    }, 3000);
}

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
    stopPolling();
});
