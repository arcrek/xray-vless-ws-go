// State Variables
let lastLogId = 0;
let allLogs = [];
let vlessData = null;
let isAutoScroll = true;
let statsInterval = null;
let logsInterval = null;
let vlessInterval = null;
let modalCurrentContent = "";

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
    toast.textContent = (type === "success" ? "✓ " : "⚠️ ") + message;

    container.appendChild(toast);
    setTimeout(() => {
        toast.style.opacity = "0";
        toast.style.transform = "translateY(8px)";
        toast.style.transition = "all 0.25s ease-out";
        setTimeout(() => toast.remove(), 250);
    }, 3000);
}

// Copy Helper
async function copyToClipboard(text, successMsg = "Đã sao chép vào clipboard!") {
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
        showToast("Không thể sao chép tự động", "error");
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
        if (!url.startsWith("vless://")) return { remark: "VLESS Node", host: "—", port: "—", path: "—", type: "ws" };
        const withoutProto = url.slice(8);
        const atIdx = withoutProto.indexOf("@");
        const questionIdx = withoutProto.indexOf("?");
        const hashIdx = withoutProto.indexOf("#");

        const uuid = atIdx > 0 ? withoutProto.substring(0, atIdx) : "";
        let hostPort = "";
        if (atIdx >= 0) {
            const endHost = questionIdx >= 0 ? questionIdx : (hashIdx >= 0 ? hashIdx : withoutProto.length);
            hostPort = withoutProto.substring(atIdx + 1, endHost);
        }

        let remark = "VLESS Node";
        if (hashIdx >= 0) {
            remark = decodeURIComponent(withoutProto.substring(hashIdx + 1));
        }

        let params = new URLSearchParams();
        if (questionIdx >= 0) {
            const queryStr = hashIdx >= 0 ? withoutProto.substring(questionIdx + 1, hashIdx) : withoutProto.substring(questionIdx + 1);
            params = new URLSearchParams(queryStr);
        }

        return {
            uuid: uuid,
            hostPort: hostPort,
            host: params.get("host") || hostPort.split(":")[0],
            port: hostPort.split(":")[1] || "443",
            path: params.get("path") || "/",
            type: params.get("type") || "ws",
            security: params.get("security") || "tls",
            sni: params.get("sni") || params.get("host") || "",
            remark: remark
        };
    } catch (e) {
        return { remark: "VLESS Node", hostPort: "—", host: "—", port: "443", path: "/", type: "ws", security: "tls" };
    }
}

// Render VLESS Nodes and Subscription QR
function renderVlessView(data) {
    vlessData = data;
    if (!data || !data.ready || !data.links || data.links.length === 0) {
        vlessNodesContainer.innerHTML = `
            <div style="color: var(--text-secondary); font-size: 13px; padding: 24px; text-align: center; border: 1px dashed var(--border-color); border-radius: var(--radius-md);">
                ⏳ Đang chờ Cloudflare Tunnel khởi tạo cấu hình VLESS...
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
        nameDiv.textContent = `⚡ ${parsed.remark}`;
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
        btnQr.textContent = "📱 Xem QR";
        btnQr.addEventListener("click", () => openQrModal(link, `Mã QR: ${parsed.remark}`));
        actionsDiv.appendChild(btnQr);

        const btnCopy = document.createElement("button");
        btnCopy.className = "btn btn-primary btn-sm";
        btnCopy.textContent = "📋 Sao chép Link";
        btnCopy.addEventListener("click", () => copyToClipboard(link, `Đã sao chép node [${parsed.remark}]!`));
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
function openQrModal(content, title = "Mã QR") {
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

    const width = rect.width;
    const height = rect.height;

    ctx.clearRect(0, 0, width, height);

    if (!history || history.length === 0) {
        ctx.fillStyle = "#6e7681";
        ctx.font = "12px sans-serif";
        ctx.fillText("Đang thu thập mẫu lưu lượng...", 15, height / 2);
        return;
    }

    let maxVal = 1024; // baseline min 1KB/s
    history.forEach(pt => {
        if (pt.up_bps > maxVal) maxVal = pt.up_bps;
        if (pt.down_bps > maxVal) maxVal = pt.down_bps;
    });

    const step = width / Math.max(history.length - 1, 1);

    // Draw Gridlines
    ctx.strokeStyle = "rgba(255, 255, 255, 0.05)";
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, height * 0.25); ctx.lineTo(width, height * 0.25);
    ctx.moveTo(0, height * 0.5); ctx.lineTo(width, height * 0.5);
    ctx.moveTo(0, height * 0.75); ctx.lineTo(width, height * 0.75);
    ctx.stroke();

    // Helper to draw smooth series
    function drawSeries(key, color, glowColor) {
        ctx.beginPath();
        history.forEach((pt, i) => {
            const val = pt[key] || 0;
            const x = i * step;
            const y = height - (val / maxVal) * (height - 16) - 8;
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        });

        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.shadowColor = glowColor;
        ctx.shadowBlur = 6;
        ctx.stroke();
        ctx.shadowBlur = 0;
    }

    // Downlink (Cyan) & Uplink (Green)
    drawSeries("down_bps", "#38bdf8", "rgba(56, 189, 248, 0.4)");
    drawSeries("up_bps", "#3fb950", "rgba(63, 185, 80, 0.4)");
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
            xrayState.textContent = "Hoạt động (Online)";
            xrayState.style.color = "var(--accent-green-bright)";
        } else {
            dotXray.className = "dot dot-off";
            xrayState.textContent = "Ngắt kết nối";
            xrayState.style.color = "var(--accent-red-bright)";
        }

        // Tunnel Status
        if (data.tunnel_ready) {
            dotTunnel.className = "dot dot-on";
            tunnelState.textContent = "Đã kết nối (Ready)";
            tunnelState.style.color = "var(--accent-green-bright)";
            overallStatusBadge.textContent = "● Tunnel Hoạt động";
            overallStatusBadge.className = "tag tag-tls";
        } else {
            dotTunnel.className = "dot dot-off";
            tunnelState.textContent = "Đang kết nối...";
            tunnelState.style.color = "var(--accent-yellow)";
            overallStatusBadge.textContent = "Đang kết nối Tunnel...";
            overallStatusBadge.className = "tag";
        }

        readyConnections.textContent = data.ready_connections || 0;
        hostnameElem.textContent = data.hostname || "Chưa có Hostname";
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
            togglePasswordBtn.textContent = "🙈";
        } else {
            passwordInput.type = "password";
            togglePasswordBtn.textContent = "👁️";
        }
    });

    // Login submit
    loginForm.addEventListener("submit", async (e) => {
        e.preventDefault();
        loginError.classList.add("hidden");
        loginSubmitBtn.disabled = true;
        loginSubmitBtn.textContent = "Đang xác thực...";

        try {
            const res = await fetch("/api/login", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ password: passwordInput.value })
            });

            const data = await res.json();
            if (res.ok && data.ok) {
                showToast("Đăng nhập thành công!", "success");
                showDashboard(true);
            } else {
                loginError.textContent = data.error || "Mật khẩu không chính xác!";
                loginError.classList.remove("hidden");
                passwordInput.select();
            }
        } catch (err) {
            loginError.textContent = "Không thể kết nối đến máy chủ!";
            loginError.classList.remove("hidden");
        } finally {
            loginSubmitBtn.disabled = false;
            loginSubmitBtn.textContent = "Đăng nhập";
        }
    });

    // Logout
    btnLogout.addEventListener("click", async () => {
        try {
            await fetch("/api/logout", { method: "POST" });
            showToast("Đã đăng xuất", "success");
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
        showToast("Đã làm mới dữ liệu", "success");
    });

    // Copy Hostname
    copyHostnameBtn.addEventListener("click", () => {
        copyToClipboard(hostnameElem.textContent, "Đã sao chép Hostname!");
    });

    // Copy Base64 Subscription
    btnCopySubB64.addEventListener("click", () => {
        if (vlessData && vlessData.base64_config) {
            copyToClipboard(vlessData.base64_config, "Đã sao chép chuỗi Base64 Subscription!");
        }
    });

    // Copy Raw Config
    btnCopyRawConfig.addEventListener("click", () => {
        if (vlessData && vlessData.raw_config) {
            copyToClipboard(vlessData.raw_config, "Đã sao chép nội dung frp_info.config!");
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
            showToast("Đã tải xuống file frp_info.config", "success");
        }
    });

    // QR Preview Box Zoom Click
    qrPreviewBox.addEventListener("click", () => {
        if (vlessData && vlessData.base64_config) {
            openQrModal(vlessData.base64_config, "Mã QR Subscription (Base64)");
        }
    });

    // Modal Close
    btnCloseModal.addEventListener("click", closeQrModal);
    btnModalCloseFooter.addEventListener("click", closeQrModal);
    qrModal.addEventListener("click", (e) => {
        if (e.target === qrModal) closeQrModal();
    });

    btnModalCopy.addEventListener("click", () => {
        copyToClipboard(modalCurrentContent, "Đã sao chép nội dung mã QR!");
    });

    // Log Controls
    logSearch.addEventListener("input", applyLogFilters);
    logFilterSource.addEventListener("change", applyLogFilters);
    logFilterType.addEventListener("change", applyLogFilters);

    btnToggleScroll.addEventListener("click", () => {
        isAutoScroll = !isAutoScroll;
        btnToggleScroll.textContent = isAutoScroll ? "⬇️ Auto-scroll: Bật" : "⏸️ Auto-scroll: Tắt";
        btnToggleScroll.className = isAutoScroll ? "btn btn-sm" : "btn btn-sm btn-danger";
    });

    btnClearLogs.addEventListener("click", () => {
        logContainer.innerHTML = `<div style="color: var(--text-muted); font-style: italic; padding: 10px;">Đã xóa nhật ký hiển thị.</div>`;
        showToast("Đã xóa nhật ký trên giao diện", "success");
    });

    btnCopyLogs.addEventListener("click", () => {
        const entries = logContainer.querySelectorAll(".log-entry");
        const lines = [];
        entries.forEach(e => {
            if (e.style.display !== "none") {
                lines.push(e.textContent.trim());
            }
        });
        copyToClipboard(lines.join("\n"), "Đã sao chép toàn bộ nhật ký hiển thị!");
    });
}

// Window Onload
document.addEventListener("DOMContentLoaded", () => {
    initEventListeners();
    checkAuthStatus();
});
