// --- Global State ---
let currentConfig = null;
let trafficChart = null;
let lastTotalRequests = 0;
let startTime = Date.now();

// --- Initialization ---
document.addEventListener('DOMContentLoaded', () => {
    initCharts();
    loadConfig();
    startPolling();
    switchTab('dashboard'); // Default tab
});

// --- Navigation ---
function switchTab(tabId) {
    // 1. Update Sidebar
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
    document.querySelector(`.nav-item[onclick="switchTab('${tabId}')"]`).classList.add('active');

    // 2. Show View
    document.querySelectorAll('.view').forEach(el => el.classList.remove('active'));
    document.getElementById(`view-${tabId}`).classList.add('active');

    // 3. Update Header
    const titles = {
        'dashboard': 'Overview',
        'rules': 'Rules Engine',
        'settings': 'Server Configuration'
    };
    document.getElementById('page-title').innerText = titles[tabId];

    // 4. Load Data if needed
    if (tabId === 'rules') loadRules();
    if (tabId === 'settings') loadConfig();
}

// --- Charts ---
function initCharts() {
    const ctx = document.getElementById('trafficChart').getContext('2d');

    // Gradient
    const gradient = ctx.createLinearGradient(0, 0, 0, 300);
    gradient.addColorStop(0, '#3b82f6');
    gradient.addColorStop(1, 'rgba(59, 130, 246, 0)');

    trafficChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: Array(30).fill(''),
            datasets: [{
                label: 'Requests',
                data: Array(30).fill(0),
                borderColor: '#60a5fa',
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
            plugins: { legend: { display: false } },
            scales: {
                x: { display: false },
                y: {
                    beginAtZero: true,
                    grid: { color: '#27272a' },
                    ticks: { color: '#71717a' }
                }
            },
            animation: { duration: 0 }
        }
    });
}

// --- Data Polling ---
function startPolling() {
    setInterval(fetchData, 2000);
    fetchData(); // Initial call
}

async function fetchData() {
    try {
        const [vars, stats, logs] = await Promise.all([
            fetch('/debug/vars').then(r => r.json()),
            fetch('/api/stats').then(r => r.json()),
            fetch('/api/logs').then(r => r.json())
        ]);

        updateDashboard(vars, stats, logs);
        document.getElementById('ctx-status').innerText = "CONNECTED";
        document.getElementById('ctx-status').style.color = "#10b981";
    } catch (e) {
        console.error("Poll Error:", e);
        document.getElementById('ctx-status').innerText = "OFFLINE";
        document.getElementById('ctx-status').style.color = "#ef4444";
    }
}

function updateDashboard(vars, stats, logs) {
    const total = vars.requests_total || 0;
    const blocked = vars.requests_blocked || 0;
    const latencyTotal = vars.latency_total_ms || 0;

    // RPS Calc
    const delta = total - lastTotalRequests;
    // But poll is 2s, so RPS approx = delta / 2
    // If first run, delta is huge, ignore
    if (lastTotalRequests !== 0) {
        document.getElementById('rps').innerText = Math.round(delta / 2);

        // Update Chart
        trafficChart.data.datasets[0].data.push(delta);
        trafficChart.data.datasets[0].data.shift();
        trafficChart.update();
    }
    lastTotalRequests = total;

    document.getElementById('blocked-count').innerText = blocked;
    document.getElementById('latency').innerText = total > 0 ? (latencyTotal / total).toFixed(1) : '0';

    // Uptime
    const uptimeSec = Math.floor((Date.now() - startTime) / 1000);
    const hrs = Math.floor(uptimeSec / 3600);
    const mins = Math.floor((uptimeSec % 3600) / 60);
    document.getElementById('uptime').innerText = `${hrs}h ${mins}m`;

    // Logs
    const tbody = document.querySelector('#logs-table tbody');
    tbody.innerHTML = '';
    logs.slice(0, 10).forEach(log => { // Last 10
        const tr = document.createElement('tr');

        // Status Color
        let statusColor = "text-success";
        if (log.status_code >= 400 && log.status_code < 500) statusColor = "text-warning";
        if (log.status_code >= 500) statusColor = "text-danger";

        // Action Style
        let actionStyle = log.action === "BLOCKED" ?
            '<span class="badge badge-danger">BLOCKED</span>' :
            '<span class="badge badge-success">ALLOWED</span>';

        tr.innerHTML = `
            <td class="font-mono text-muted">${new Date(log.timestamp).toLocaleTimeString()}</td>
            <td class="font-bold">${log.method}</td>
            <td class="font-mono text-primary">${log.path}</td>
            <td class="font-mono">${log.client_ip}</td>
            <td class="${statusColor} font-bold">${log.status_code}</td>
            <td>${actionStyle}</td>
        `;
        tbody.appendChild(tr);
    });
}

// --- Configuration ---
async function loadConfig() {
    try {
        const res = await fetch('/api/config');
        const cfg = await res.json();
        currentConfig = cfg;

        // Populate inputs
        document.getElementById('cfg-port').value = cfg.server.port;
        document.getElementById('cfg-targets').value = cfg.proxy.targets.join(', ');
        document.getElementById('cfg-ratelimit').value = cfg.security.rate_limit.requests_per_minute;
        document.getElementById('cfg-maxbody').value = cfg.security.max_body_size || 0;

    } catch (e) {
        console.error("Failed to load config", e);
    }
}

async function saveConfig(e) {
    e.preventDefault();
    if (!currentConfig) return;

    // Read form
    // Note: Config structure must match backend JSON
    const newCfg = JSON.parse(JSON.stringify(currentConfig)); // Deep copy

    newCfg.server.port = document.getElementById('cfg-port').value;

    const targetsStr = document.getElementById('cfg-targets').value;
    newCfg.proxy.targets = targetsStr.split(',').map(s => s.trim().replace(/\/$/, '')).filter(s => s);

    newCfg.security.rate_limit.requests_per_minute = parseInt(document.getElementById('cfg-ratelimit').value);
    newCfg.security.max_body_size = parseInt(document.getElementById('cfg-maxbody').value);

    try {
        const res = await fetch('/api/config', {
            method: 'POST',
            body: JSON.stringify(newCfg),
            headers: { 'Content-Type': 'application/json' }
        });
        const result = await res.json();
        if (res.ok) {
            alert("Configuration Saved!");
        } else {
            alert("Error: " + result);
        }
    } catch (err) {
        alert("Failed to save: " + err);
    }
}

async function loadRules() {
    try {
        // We can get rules from currentConfig or fetch /api/config again
        const res = await fetch('/api/config');
        const cfg = await res.json();
        const rules = cfg.security.rules || [];

        const container = document.getElementById('rules-container');
        container.innerHTML = '';

        rules.forEach((rule, idx) => {
            const el = document.createElement('div');
            el.className = 'rule-card';
            el.innerHTML = `
                <div class="rule-header">
                    <span class="rule-name">${rule.name}</span>
                    <span class="badge badge-success">ACTIVE</span>
                </div>
                <div class="rule-pattern">${rule.pattern}</div>
                <div class="rule-meta">
                    <span>Target: ${rule.location}</span>
                </div>
            `;
            container.appendChild(el);
        });
    } catch (e) {
        console.error("Load rules failed", e);
    }
}
