// State Variables
let lastLogId = 0;
let allLogs = [];
let vlessData = null;
let isAutoScroll = true;
let statsInterval = null;
let logsInterval = null;
let vlessInterval = null;
let modalCurrentContent = "";

// SVG Icon Templates (inline, no external dependencies)
const ICONS = {
    check: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
    alertCircle: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>',
    copy: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>',
    qr: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="8" height="8" rx="1"/><rect x="14" y="2" width="8" height="8" rx="1"/><rect x="2" y="14" width="8" height="8" rx="1"/><rect x="14" y="14" width="4" height="4" rx="1"/><line x1="22" y1="18" x2="22" y2="22"/><line x1="18" y1="22" x2="22" y2="22"/></svg>',
    bolt: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>',
    eye: '<svg class="icon-eye" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>',
    eyeOff: '<svg class="icon-eye" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>',
    chevronDown: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>',
};

// DOM Elements
const loginView = document.getElementById("login-view");
const dashboardView = document.getElementById("dashboard-view");
const loginForm = document.getElementById("login-form");
const passwordInput = document.getElementById("password-input");
const togglePasswordBtn = document.getElementById("toggle-password");
const loginError = document.getElementById("login-error");
const loginSubmitBtn = document.getElementById("login-submit-btn");
const btnLogout = document.getElementById("btn-logout");
const btnRefresh = document.getElementById("btn-refresh");

// Status Elements
const dotXray = document.getElementById("dot-xray");
const xrayState = document.getElementById("xray-state");
const dotTunnel = document.getElementById("dot-tunnel");
const tunnelState = document.getElementById("tunnel-state");
const readyConnections = document.getElementById("ready-connections");
const hostnameElem = document.getElementById("hostname");
const publicIpElem = document.getElementById("public-ip");
const uptimeElem = document.getElementById("uptime");
const overallStatusBadge = document.getElementById("overall-status-badge");
const copyHostnameBtn = document.getElementById("copy-hostname-btn");

// Throughput Elements
const upBpsElem = document.getElementById("up-bps");
const downBpsElem = document.getElementById("down-bps");
const upTotalElem = document.getElementById("up-total");
const downTotalElem = document.getElementById("down-total");
const sparklineCanvas = document.getElementById("sparkline");

// VLESS & Subscription Elements
const vlessNodesContainer = document.getElementById("vless-nodes-container");
const subQrCanvas = document.getElementById("sub-qr-canvas");
const qrPreviewBox = document.getElementById("qr-preview-box");
const btnCopySubB64 = document.getElementById("btn-copy-sub-b64");
const btnCopyRawConfig = document.getElementById("btn-copy-raw-config");
const btnDownloadConfig = document.getElementById("btn-download-config");

// Logs Elements
const logContainer = document.getElementById("log-container");
const logSearch = document.getElementById("log-search");
const logFilterSource = document.getElementById("log-filter-source");
const logFilterType = document.getElementById("log-filter-type");
const btnToggleScroll = document.getElementById("btn-toggle-scroll");
const btnClearLogs = document.getElementById("btn-clear-logs");
const btnCopyLogs = document.getElementById("btn-copy-logs");

// Modal Elements
const qrModal = document.getElementById("qr-modal");
const modalQrCanvas = document.getElementById("modal-qr-canvas");
const modalQrTitle = document.getElementById("modal-qr-title");
const modalQrDesc = document.getElementById("modal-qr-desc");
const btnCloseModal = document.getElementById("btn-close-modal");
const btnModalCloseFooter = document.getElementById("btn-modal-close-footer");
const btnModalCopy = document.getElementById("btn-modal-copy");

// Toast Notification
function showToast(message, type = "success") {
    const container = document.getElementById("toast-container");
    if (!container) return;

    const toast = document.createElement("div");
    toast.className = `toast toast-${type}`;

    const iconSpan = document.createElement("span");
    iconSpan.className = "toast-icon";
    iconSpan.innerHTML = type === "success" ? ICONS.check : ICONS.alertCircle;
    toast.appendChild(iconSpan);

    const textSpan = document.createElement("span");
    textSpan.textContent = message;
    toast.appendChild(textSpan);

    container.appendChild(toast);
    setTimeout(() => {
        toast.style.opacity = "0";
        toast.style.transform = "translateY(8px)";
        toast.style.transition = "all 0.25s ease-out";
        setTimeout(() => toast.remove(), 250);
    }, 3000);
}

// Copy Helper
async function copyToClipboard(text, successMsg = "Da sao chep vao clipboard!") {
    if (!text) return;
    try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(text);
        } else {
            const input = document.createElement("textarea");
            input.value = text;
            input.style.position = "fixed";
            input.style.opacity = "0";
            document.body.appendChild(input);
            input.select();
            document.execCommand("copy");
            document.body.removeChild(input);
        }
        showToast(successMsg, "success");
    } catch (e) {
        console.error("Copy failed:", e);
        showToast("Khong the sao chep tu dong", "error");
    }
}

// Format Units Helper
function formatBytes(bytes) {
    if (bytes === 0 || isNaN(bytes)) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

function formatBps(n) {
    return formatBytes(n) + "/s";
}

function formatUptime(sec) {
    if (!sec || isNaN(sec)) return "00:00:00";
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    return `${String(h).padStart(2, "0")}h ${String(m).padStart(2, "0")}m ${String(s).padStart(2, "0")}s`;
}

// Parse VLESS URL for human readable display
function parseVlessUrl(url) {
    try {
        if (!url.startsWith("vless://")) return { remark: url, host: "", port: "", security: "", type: "" };
        const withoutProto = url.substring(8);
        const hashIdx = withoutProto.lastIndexOf("#");
        let remark = hashIdx > -1 ? decodeURIComponent(withoutProto.substring(hashIdx + 1)) : "Node";
        const mainPart = hashIdx > -1 ? withoutProto.substring(0, hashIdx) : withoutProto;
        const atIdx = mainPart.indexOf("@");
        const hostPortQuery = mainPart.substring(atIdx + 1);
        const qIdx = hostPortQuery.indexOf("?");
        const hostPort = qIdx > -1 ? hostPortQuery.substring(0, qIdx) : hostPortQuery;
        const queryStr = qIdx > -1 ? hostPortQuery.substring(qIdx + 1) : "";
        const parts = hostPort.split(":");
        const host = parts[0];
        const port = parts[1] || "443";
        const params = new URLSearchParams(queryStr);
        return {
            remark: remark,
            host: host,
            port: port,
            security: params.get("security") || "tls",
            type: params.get("type") || "ws",
            sni: params.get("sni") || "",
            path: params.get("path") || "",
        };
    } catch (e) {
        return { remark: url, host: "", port: "", security: "", type: "" };
    }
}

// Render VLESS Nodes and Subscription QR
function renderVlessView(data) {
    vlessData = data;
    if (!data || !data.ready || !data.links || data.links.length === 0) {
        vlessNodesContainer.innerHTML = `
            <div class="vless-placeholder" style="border: 1px dashed var(--border-color); border-radius: var(--radius-md); padding: 24px;">
                Dang cho Cloudflare Tunnel khoi tao cau hinh VLESS...
            </div>
        `;
        const ctx = subQrCanvas.getContext("2d");
        ctx.clearRect(0, 0, subQrCanvas.width, subQrCanvas.height);
        return;
    }

    // Render Nodes
    vlessNodesContainer.innerHTML = "";
    data.links.forEach((link, idx) => {
        const parsed = parseVlessUrl(link);
        const card = document.createElement("div");
        card.className = "vless-node-card";

        const headerDiv = document.createElement("div");
        headerDiv.className = "node-header";

        const nameDiv = document.createElement("div");
        nameDiv.className = "node-name";
        nameDiv.innerHTML = ICONS.bolt + " ";
        nameDiv.appendChild(document.createTextNode(parsed.remark));
        headerDiv.appendChild(nameDiv);

        const tagsDiv = document.createElement("div");
        tagsDiv.className = "node-tags";

        const tagTls = document.createElement("span");
        tagTls.className = "tag tag-tls";
        tagTls.textContent = "TLS";
        tagsDiv.appendChild(tagTls);

        const tagType = document.createElement("span");
        tagType.className = "tag";
        tagType.textContent = (parsed.type || "ws").toUpperCase();
        tagsDiv.appendChild(tagType);

        const tagPort = document.createElement("span");
        tagPort.className = "tag";
        tagPort.textContent = `Port ${parsed.port}`;
        tagsDiv.appendChild(tagPort);

        headerDiv.appendChild(tagsDiv);
        card.appendChild(headerDiv);

        const urlBox = document.createElement("div");
        urlBox.className = "node-url-box";
        urlBox.title = link;
        urlBox.textContent = link;
        card.appendChild(urlBox);

        const actionsDiv = document.createElement("div");
        actionsDiv.className = "node-actions";

        const btnQr = document.createElement("button");
        btnQr.className = "btn btn-sm";
        btnQr.innerHTML = ICONS.qr + " Xem QR";
        btnQr.addEventListener("click", () => openQrModal(link, `Ma QR: ${parsed.remark}`));
        actionsDiv.appendChild(btnQr);

        const btnCopy = document.createElement("button");
        btnCopy.className = "btn btn-primary btn-sm";
        btnCopy.innerHTML = ICONS.copy + " Sao chep Link";
        btnCopy.addEventListener("click", () => copyToClipboard(link, `Da sao chep node [${parsed.remark}]!`));
        actionsDiv.appendChild(btnCopy);

        card.appendChild(actionsDiv);
        vlessNodesContainer.appendChild(card);
    });


    // Render Base64 Subscription QR Code
    if (data.base64_config && window.QRCode) {
        try {
            QRCode.renderCanvas(data.base64_config, subQrCanvas, {
                cellSize: 4,
                margin: 2,
                dark: "#000000",
                light: "#ffffff"
            });
        } catch (e) {
            console.error("QR render error:", e);
        }
    }
}

// Modal Handling
function openQrModal(content, title = "Ma QR") {
    modalCurrentContent = content;
    modalQrTitle.textContent = title;
    modalQrDesc.textContent = content;

    if (window.QRCode) {
        try {
            QRCode.renderCanvas(content, modalQrCanvas, {
                cellSize: 6,
                margin: 3,
                dark: "#000000",
                light: "#ffffff"
            });
        } catch (e) {
            console.error("Modal QR render error:", e);
        }
    }

    qrModal.classList.remove("hidden");
}

function closeQrModal() {
    qrModal.classList.add("hidden");
}

// Log Line Builder
function buildLogEntry(item) {
    const type = (item.type || "INFO").toUpperCase();
    const source = (item.source || "XRAY").toUpperCase();

    let typeClass = "log-type-info";
    if (type.includes("ERROR")) typeClass = "log-type-error";
    if (type.includes("SUCCESS")) typeClass = "log-type-success";
    if (type.includes("WARN")) typeClass = "log-type-warn";

    let badgeClass = "badge-sys";
    if (source.includes("XRAY")) badgeClass = "badge-xray";
    if (source.includes("CLOUDFLARE")) badgeClass = "badge-cloudflare";
    if (source.includes("CI")) badgeClass = "badge-ci";

    const entry = document.createElement("div");
    entry.className = "log-entry";
    entry.dataset.source = source;
    entry.dataset.type = type;

    const timeSpan = document.createElement("span");
    timeSpan.className = "log-time";
    timeSpan.textContent = item.time ? `[${item.time}]` : "";
    entry.appendChild(timeSpan);

    const badgeSpan = document.createElement("span");
    badgeSpan.className = `log-badge ${badgeClass}`;
    badgeSpan.textContent = source;
    entry.appendChild(badgeSpan);

    const typeSpan = document.createElement("span");
    typeSpan.className = `log-badge ${typeClass}`;
    typeSpan.textContent = type;
    entry.appendChild(typeSpan);

    const msgSpan = document.createElement("span");
    msgSpan.className = "log-message";
    if (typeof item.text === "object") {
        msgSpan.textContent = JSON.stringify(item.text);
    } else {
        msgSpan.textContent = String(item.text);
    }
    entry.appendChild(msgSpan);

    return entry;
}

// Draw Sparkline with Smooth Gradient Canvas
function drawSparkline(history) {
    if (!sparklineCanvas) return;
    const ctx = sparklineCanvas.getContext("2d");
    const dpr = window.devicePixelRatio || 1;
    const rect = sparklineCanvas.getBoundingClientRect();
    sparklineCanvas.width = rect.width * dpr;
    sparklineCanvas.height = rect.height * dpr;
    ctx.scale(dpr, dpr);

    const W = rect.width;
    const H = rect.height;
    ctx.clearRect(0, 0, W, H);

    if (!history || history.length < 2) {
        ctx.fillStyle = "#6e7681";
        ctx.font = "12px sans-serif";
        ctx.textAlign = "center";
        ctx.fillText("Dang thu thap du lieu...", W / 2, H / 2);
        return;
    }

    const upVals = history.map(h => h.up || 0);
    const downVals = history.map(h => h.down || 0);
    const maxVal = Math.max(...upVals, ...downVals, 1);
    const padding = { top: 8, bottom: 8, left: 0, right: 0 };
    const chartW = W - padding.left - padding.right;
    const chartH = H - padding.top - padding.bottom;

    function drawLine(values, color, fillColor) {
        const len = values.length;
        const step = chartW / (len - 1);

        ctx.beginPath();
        for (let i = 0; i < len; i++) {
            const x = padding.left + i * step;
            const y = padding.top + chartH - (values[i] / maxVal) * chartH;
            if (i === 0) ctx.moveTo(x, y);
            else {
                const prevX = padding.left + (i - 1) * step;
                const prevY = padding.top + chartH - (values[i - 1] / maxVal) * chartH;
                const cpX = (prevX + x) / 2;
                ctx.bezierCurveTo(cpX, prevY, cpX, y, x, y);
            }
        }

        // Fill gradient
        const gradient = ctx.createLinearGradient(0, padding.top, 0, H);
        gradient.addColorStop(0, fillColor);
        gradient.addColorStop(1, "transparent");

        ctx.lineTo(padding.left + (len - 1) * step, H);
        ctx.lineTo(padding.left, H);
        ctx.closePath();
        ctx.fillStyle = gradient;
        ctx.fill();

        // Stroke line
        ctx.beginPath();
        for (let i = 0; i < len; i++) {
            const x = padding.left + i * step;
            const y = padding.top + chartH - (values[i] / maxVal) * chartH;
            if (i === 0) ctx.moveTo(x, y);
            else {
                const prevX = padding.left + (i - 1) * step;
                const prevY = padding.top + chartH - (values[i - 1] / maxVal) * chartH;
                const cpX = (prevX + x) / 2;
                ctx.bezierCurveTo(cpX, prevY, cpX, y, x, y);
            }
        }
        ctx.strokeStyle = color;
        ctx.lineWidth = 1.5;
        ctx.stroke();
    }

    drawLine(upVals, "#3fb950", "rgba(63, 185, 80, 0.12)");
    drawLine(downVals, "#38bdf8", "rgba(56, 189, 248, 0.12)");
}

// Fetch Stats
async function fetchStats() {
    try {
        const res = await fetch("/stats");
        if (res.status === 401) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) return;

        const data = await res.json();

        // Xray Status
        if (data.xray_up) {
            dotXray.className = "dot dot-on";
            xrayState.textContent = "Hoat dong (Online)";
            xrayState.style.color = "var(--accent-green-bright)";
        } else {
            dotXray.className = "dot dot-off";
            xrayState.textContent = "Ngat ket noi";
            xrayState.style.color = "var(--accent-red-bright)";
        }

        // Tunnel Status
        if (data.tunnel_ready) {
            dotTunnel.className = "dot dot-on";
            tunnelState.textContent = "Da ket noi (Ready)";
            tunnelState.style.color = "var(--accent-green-bright)";
            overallStatusBadge.textContent = "Tunnel Hoat dong";
            overallStatusBadge.className = "tag tag-tls";
        } else {
            dotTunnel.className = "dot dot-off";
            tunnelState.textContent = "Dang ket noi...";
            tunnelState.style.color = "var(--accent-yellow)";
            overallStatusBadge.textContent = "Dang ket noi Tunnel...";
            overallStatusBadge.className = "tag";
        }

        readyConnections.textContent = data.ready_connections || 0;
        hostnameElem.textContent = data.hostname || "Chua co Hostname";
        uptimeElem.textContent = formatUptime(data.uptime_sec);

        upBpsElem.textContent = formatBps(data.uplink_bps);
        downBpsElem.textContent = formatBps(data.downlink_bps);
        upTotalElem.textContent = formatBytes(data.uplink_total);
        downTotalElem.textContent = formatBytes(data.downlink_total);

        drawSparkline(data.history);
    } catch (e) {
        console.error("fetchStats error:", e);
    }
}

// Fetch VLESS Config
async function fetchVlessInfo() {
    try {
        const res = await fetch("/api/vless-info");
        if (res.status === 401) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) return;

        const data = await res.json();
        renderVlessView(data);

        if (data.ip) {
            publicIpElem.textContent = `IP Public: ${data.ip}`;
        }
    } catch (e) {
        console.error("fetchVlessInfo error:", e);
    }
}

// Fetch Realtime Logs
async function fetchLogs() {
    try {
        const res = await fetch(`/logs?last_id=${lastLogId}`);
        if (res.status === 401) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) return;

        const data = await res.json();
        if (data.new_logs && data.new_logs.length > 0) {
            if (lastLogId === 0) {
                logContainer.innerHTML = "";
            }
            lastLogId = data.last_id;

            data.new_logs.forEach(log => {
                allLogs.push(log);
                if (allLogs.length > 500) allLogs.shift();
                const node = buildLogEntry(log);
                logContainer.appendChild(node);
            });

            applyLogFilters();

            if (isAutoScroll) {
                logContainer.scrollTop = logContainer.scrollHeight;
            }
        }
    } catch (e) {
        console.error("fetchLogs error:", e);
    }
}

// Filter Logs
function applyLogFilters() {
    const searchVal = logSearch.value.trim().toLowerCase();
    const sourceVal = logFilterSource.value;
    const typeVal = logFilterType.value;

    const entries = logContainer.querySelectorAll(".log-entry");
    entries.forEach(entry => {
        const source = entry.dataset.source || "";
        const type = entry.dataset.type || "";
        const text = entry.textContent.toLowerCase();

        let visible = true;
        if (sourceVal !== "ALL" && !source.includes(sourceVal)) visible = false;
        if (typeVal !== "ALL" && !type.includes(typeVal)) visible = false;
        if (searchVal && !text.includes(searchVal)) visible = false;

        entry.style.display = visible ? "flex" : "none";
    });
}

// Auth State Check
async function checkAuthStatus() {
    try {
        const res = await fetch("/api/auth-status");
        if (!res.ok) return;

        const data = await res.json();
        if (!data.auth_required || data.authenticated) {
            showDashboard(data.auth_required);
        } else {
            showLogin();
        }
    } catch (e) {
        console.error("checkAuthStatus error:", e);
        showDashboard(false);
    }
}

function handleUnauthorized() {
    stopPolling();
    showLogin();
}

function showLogin() {
    loginView.classList.remove("hidden");
    dashboardView.classList.add("hidden");
    passwordInput.value = "";
    loginError.classList.add("hidden");
    passwordInput.focus();
}

function showDashboard(authRequired) {
    loginView.classList.add("hidden");
    dashboardView.classList.remove("hidden");

    if (authRequired) {
        btnLogout.classList.remove("hidden");
    } else {
        btnLogout.classList.add("hidden");
    }

    startPolling();
}

function startPolling() {
    stopPolling();
    fetchStats();
    fetchVlessInfo();
    fetchLogs();

    statsInterval = setInterval(fetchStats, 2000);
    vlessInterval = setInterval(fetchVlessInfo, 10000);
    logsInterval = setInterval(fetchLogs, 1000);
}

function stopPolling() {
    if (statsInterval) clearInterval(statsInterval);
    if (vlessInterval) clearInterval(vlessInterval);
    if (logsInterval) clearInterval(logsInterval);
}

// Event Listeners Initialization
function initEventListeners() {
    // Password toggle
    togglePasswordBtn.addEventListener("click", () => {
        if (passwordInput.type === "password") {
            passwordInput.type = "text";
            togglePasswordBtn.innerHTML = ICONS.eyeOff;
        } else {
            passwordInput.type = "password";
            togglePasswordBtn.innerHTML = ICONS.eye;
        }
    });

    // Login submit
    loginForm.addEventListener("submit", async (e) => {
        e.preventDefault();
        loginError.classList.add("hidden");
        loginSubmitBtn.disabled = true;
        loginSubmitBtn.textContent = "Dang xac thuc...";

        try {
            const res = await fetch("/api/login", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ password: passwordInput.value })
            });

            const data = await res.json();
            if (res.ok && data.ok) {
                showToast("Dang nhap thanh cong!", "success");
                showDashboard(true);
            } else {
                loginError.textContent = data.error || "Mat khau khong chinh xac!";
                loginError.classList.remove("hidden");
                passwordInput.select();
            }
        } catch (err) {
            loginError.textContent = "Khong the ket noi den may chu!";
            loginError.classList.remove("hidden");
        } finally {
            loginSubmitBtn.disabled = false;
            loginSubmitBtn.textContent = "Dang nhap";
        }
    });

    // Logout
    btnLogout.addEventListener("click", async () => {
        try {
            await fetch("/api/logout", { method: "POST" });
            showToast("Da dang xuat", "success");
            showLogin();
        } catch (e) {
            showLogin();
        }
    });

    // Refresh button
    btnRefresh.addEventListener("click", () => {
        fetchStats();
        fetchVlessInfo();
        fetchLogs();
        showToast("Da lam moi du lieu", "success");
    });

    // Copy Hostname
    copyHostnameBtn.addEventListener("click", () => {
        copyToClipboard(hostnameElem.textContent, "Da sao chep Hostname!");
    });

    // Copy Base64 Subscription
    btnCopySubB64.addEventListener("click", () => {
        if (vlessData && vlessData.base64_config) {
            copyToClipboard(vlessData.base64_config, "Da sao chep chuoi Base64 Subscription!");
        }
    });

    // Copy Raw Config
    btnCopyRawConfig.addEventListener("click", () => {
        if (vlessData && vlessData.raw_config) {
            copyToClipboard(vlessData.raw_config, "Da sao chep noi dung frp_info.config!");
        }
    });

    // Download Config
    btnDownloadConfig.addEventListener("click", () => {
        if (vlessData && vlessData.raw_config) {
            const blob = new Blob([vlessData.raw_config], { type: "text/plain;charset=utf-8" });
            const url = URL.createObjectURL(blob);
            const a = document.createElement("a");
            a.href = url;
            a.download = "frp_info.config";
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            showToast("Da tai xuong file frp_info.config", "success");
        }
    });

    // QR Preview Box Zoom Click
    qrPreviewBox.addEventListener("click", () => {
        if (vlessData && vlessData.base64_config) {
            openQrModal(vlessData.base64_config, "Ma QR Subscription (Base64)");
        }
    });

    // Modal Close
    btnCloseModal.addEventListener("click", closeQrModal);
    btnModalCloseFooter.addEventListener("click", closeQrModal);
    qrModal.addEventListener("click", (e) => {
        if (e.target === qrModal) closeQrModal();
    });

    btnModalCopy.addEventListener("click", () => {
        copyToClipboard(modalCurrentContent, "Da sao chep noi dung ma QR!");
    });

    // Log Controls
    logSearch.addEventListener("input", applyLogFilters);
    logFilterSource.addEventListener("change", applyLogFilters);
    logFilterType.addEventListener("change", applyLogFilters);

    btnToggleScroll.addEventListener("click", () => {
        isAutoScroll = !isAutoScroll;
        btnToggleScroll.innerHTML = (isAutoScroll ? ICONS.chevronDown : ICONS.chevronDown) + (isAutoScroll ? " Auto-scroll" : " Paused");
        btnToggleScroll.className = isAutoScroll ? "btn btn-sm" : "btn btn-sm btn-danger";
    });

    btnClearLogs.addEventListener("click", () => {
        logContainer.innerHTML = `<div class="log-placeholder">Da xoa nhat ky hien thi.</div>`;
        showToast("Da xoa nhat ky tren giao dien", "success");
    });

    btnCopyLogs.addEventListener("click", () => {
        const entries = logContainer.querySelectorAll(".log-entry");
        const lines = [];
        entries.forEach(e => {
            if (e.style.display !== "none") {
                lines.push(e.textContent.trim());
            }
        });
        copyToClipboard(lines.join("\n"), "Da sao chep toan bo nhat ky hien thi!");
    });
}

// Window Onload
document.addEventListener("DOMContentLoaded", () => {
    initEventListeners();
    checkAuthStatus();
});
