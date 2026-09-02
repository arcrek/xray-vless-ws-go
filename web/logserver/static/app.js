// ==========================================================================
// Xray VLESS Dashboard - Core Frontend Application Logic
// Zero Emoji, Clean Vietnamese Copy, HiDPI Visuals & Responsive Controls
// ==========================================================================

// Application State
let lastLogId = 0;
let allLogs = [];
let vlessData = null;
let isAutoScroll = true;
let statsInterval = null;
let logsInterval = null;
let vlessInterval = null;
let modalCurrentContent = "";

// High-fidelity SVG Icon Templates (Lucide style, stroke-width 2px)
const ICONS = {
    check: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
    alertCircle: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>',
    copy: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>',
    qr: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="8" height="8" rx="1"/><rect x="14" y="2" width="8" height="8" rx="1"/><rect x="2" y="14" width="8" height="8" rx="1"/><rect x="14" y="14" width="4" height="4" rx="1"/><line x1="22" y1="18" x2="22" y2="22"/><line x1="18" y1="22" x2="22" y2="22"/></svg>',
    server: '<svg class="icon" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>',
    eye: '<svg class="icon-eye" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>',
    eyeOff: '<svg class="icon-eye" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>',
    pause: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>',
    play: '<svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>',
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

// ── Toast Notification System ─────────────────────────────
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
    }, 3200);
}

// ── Clipboard Copy Helper ─────────────────────────────────
async function copyToClipboard(text, successMsg = "Đã sao chép vào bộ nhớ tạm!") {
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
        showToast("Không thể tự động sao chép", "error");
    }
}

// ── Formatting Helpers ────────────────────────────────────
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

// ── VLESS URL Parser (Strict Zero-Emoji & Parameter Extraction) ───
function parseVlessUrl(url) {
    try {
        if (!url || !url.startsWith("vless://")) {
            return { remark: url || "VLESS Node", host: "", port: "", security: "none", type: "ws", sni: "", path: "", hostHeader: "" };
        }
        const withoutProto = url.substring(8);
        const hashIdx = withoutProto.lastIndexOf("#");
        let rawRemark = hashIdx > -1 ? decodeURIComponent(withoutProto.substring(hashIdx + 1)) : "VLESS Node";
        
        // Strip all unicode emojis (including lightning ⚡, rocket 🚀, flags, symbols)
        const sanitizedRemark = rawRemark
            .replace(/[\u{1F300}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{1F1E0}-\u{1F1FF}\u{FE00}-\u{FE0F}]/gu, "")
            .replace(/\s+/g, " ")
            .trim() || "VLESS Node";

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
            remark: sanitizedRemark,
            host: host,
            port: port,
            security: params.get("security") || (port === "443" ? "tls" : "none"),
            type: params.get("type") || "ws",
            sni: params.get("sni") || "",
            path: params.get("path") || "",
            hostHeader: params.get("host") || "",
        };
    } catch (e) {
        return { remark: "VLESS Node", host: "", port: "", security: "none", type: "ws", sni: "", path: "", hostHeader: "" };
    }
}

// ── Render Section 2: VLESS Nodes & Base64 QR Code ─────────
function renderVlessView(data) {
    vlessData = data;
    if (!data || !data.ready || !data.links || data.links.length === 0) {
        vlessNodesContainer.innerHTML = `
            <div class="vless-placeholder">
                Đang chờ Cloudflare Tunnel hoàn tất khởi tạo cấu hình VLESS...
            </div>
        `;
        const ctx = subQrCanvas.getContext("2d");
        ctx.clearRect(0, 0, subQrCanvas.width, subQrCanvas.height);
        return;
    }

    // Render Clean Node Cards
    vlessNodesContainer.innerHTML = "";
    data.links.forEach((link, idx) => {
        const parsed = parseVlessUrl(link);
        const card = document.createElement("div");
        card.className = "vless-node-card";

        // 1. Header (Name & Badges)
        const headerDiv = document.createElement("div");
        headerDiv.className = "node-header";

        const nameDiv = document.createElement("div");
        nameDiv.className = "node-name";
        nameDiv.innerHTML = `<span class="node-name-icon">${ICONS.server}</span><span>${parsed.remark}</span>`;
        headerDiv.appendChild(nameDiv);

        const tagsDiv = document.createElement("div");
        tagsDiv.className = "node-tags";

        // Security Tag
        const isTls = parsed.security.toLowerCase() === "tls";
        const tagSec = document.createElement("span");
        tagSec.className = `tag ${isTls ? "tag-tls" : "tag-notls"}`;
        tagSec.textContent = isTls ? "TLS" : "No-TLS";
        tagsDiv.appendChild(tagSec);

        // Protocol Type Tag
        const tagType = document.createElement("span");
        tagType.className = "tag tag-ws";
        tagType.textContent = (parsed.type || "ws").toUpperCase();
        tagsDiv.appendChild(tagType);

        // Port Tag
        const tagPort = document.createElement("span");
        tagPort.className = "tag tag-port";
        tagPort.textContent = `Port ${parsed.port}`;
        tagsDiv.appendChild(tagPort);

        headerDiv.appendChild(tagsDiv);
        card.appendChild(headerDiv);

        // 2. Node Parameter Details
        if (parsed.path || parsed.sni || parsed.hostHeader) {
            const paramsRow = document.createElement("div");
            paramsRow.className = "node-params-row";

            if (parsed.path) {
                const pItem = document.createElement("div");
                pItem.className = "node-param-item";
                pItem.innerHTML = `<span class="node-param-key">Path:</span> <span class="node-param-val">${parsed.path}</span>`;
                paramsRow.appendChild(pItem);
            }
            if (parsed.sni) {
                const sItem = document.createElement("div");
                sItem.className = "node-param-item";
                sItem.innerHTML = `<span class="node-param-key">SNI:</span> <span class="node-param-val">${parsed.sni}</span>`;
                paramsRow.appendChild(sItem);
            }
            if (parsed.hostHeader && parsed.hostHeader !== parsed.sni) {
                const hItem = document.createElement("div");
                hItem.className = "node-param-item";
                hItem.innerHTML = `<span class="node-param-key">Host:</span> <span class="node-param-val">${parsed.hostHeader}</span>`;
                paramsRow.appendChild(hItem);
            }
            card.appendChild(paramsRow);
        }

        // 3. Raw URL String Box
        const urlBox = document.createElement("div");
        urlBox.className = "node-url-box";
        urlBox.title = "Bấm để chọn toàn bộ URL link";
        urlBox.textContent = link;
        card.appendChild(urlBox);

        // 4. Action Buttons
        const actionsDiv = document.createElement("div");
        actionsDiv.className = "node-actions";

        const btnQr = document.createElement("button");
        btnQr.className = "btn btn-secondary btn-sm";
        btnQr.innerHTML = `${ICONS.qr} Xem QR`;
        btnQr.title = `Xem mã QR cho cấu hình ${parsed.remark}`;
        btnQr.addEventListener("click", () => openQrModal(link, `Mã QR: ${parsed.remark}`));
        actionsDiv.appendChild(btnQr);

        const btnCopy = document.createElement("button");
        btnCopy.className = "btn btn-primary btn-sm";
        btnCopy.innerHTML = `${ICONS.copy} Sao chép Link`;
        btnCopy.title = `Sao chép URL VLESS của ${parsed.remark}`;
        btnCopy.addEventListener("click", () => copyToClipboard(link, `Đã sao chép link node [${parsed.remark}]!`));
        actionsDiv.appendChild(btnCopy);

        card.appendChild(actionsDiv);
        vlessNodesContainer.appendChild(card);
    });

    // Render Base64 Subscription QR Code Canvas
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

// ── Modal QR Code Viewer ──────────────────────────────────
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

// ── Realtime Log Item Builder ─────────────────────────────
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

// ── Sparkline Graph with HiDPI Rendering ──────────────────
function drawSparkline(history) {
    if (!sparklineCanvas) return;
    const ctx = sparklineCanvas.getContext("2d");
    const dpr = window.devicePixelRatio || 1;
    const rect = sparklineCanvas.getBoundingClientRect();
    
    // Set actual canvas size considering device pixel ratio
    sparklineCanvas.width = rect.width * dpr;
    sparklineCanvas.height = rect.height * dpr;
    ctx.scale(dpr, dpr);

    const W = rect.width;
    const H = rect.height;
    ctx.clearRect(0, 0, W, H);

    if (!history || history.length < 2) {
        ctx.fillStyle = "#64748b";
        ctx.font = "12px sans-serif";
        ctx.textAlign = "center";
        ctx.fillText("Đang thu thập dữ liệu băng thông...", W / 2, H / 2);
        return;
    }

    const upVals = history.map(h => h.up || 0);
    const downVals = history.map(h => h.down || 0);
    const maxVal = Math.max(...upVals, ...downVals, 1);
    const padding = { top: 10, bottom: 8, left: 0, right: 0 };
    const chartW = W - padding.left - padding.right;
    const chartH = H - padding.top - padding.bottom;

    function drawLine(values, strokeColor, fillColor) {
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

        // Fill area gradient
        const gradient = ctx.createLinearGradient(0, padding.top, 0, H);
        gradient.addColorStop(0, fillColor);
        gradient.addColorStop(1, "transparent");

        ctx.lineTo(padding.left + (len - 1) * step, H);
        ctx.lineTo(padding.left, H);
        ctx.closePath();
        ctx.fillStyle = gradient;
        ctx.fill();

        // Stroke line path
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
        ctx.strokeStyle = strokeColor;
        ctx.lineWidth = 1.75;
        ctx.stroke();
    }

    // Downlink (Cyan) & Uplink (Emerald Green)
    drawLine(downVals, "#38bdf8", "rgba(56, 189, 248, 0.16)");
    drawLine(upVals, "#22c55e", "rgba(34, 197, 94, 0.16)");
}

// ── Polling & API Communication ───────────────────────────
async function fetchStats() {
    try {
        const res = await fetch("/stats");
        if (res.status === 401) {
            handleUnauthorized();
            return;
        }
        if (!res.ok) return;

        const data = await res.json();

        // 1. Xray Service State
        if (data.xray_up) {
            dotXray.className = "dot dot-on";
            xrayState.textContent = "Đang hoạt động (Online)";
            xrayState.style.color = "var(--accent-green)";
        } else {
            dotXray.className = "dot dot-off";
            xrayState.textContent = "Ngắt kết nối";
            xrayState.style.color = "var(--accent-red)";
        }

        // 2. Cloudflare Tunnel State
        if (data.tunnel_ready) {
            dotTunnel.className = "dot dot-on";
            tunnelState.textContent = "Đã kết nối (Ready)";
            tunnelState.style.color = "var(--accent-green)";
            overallStatusBadge.textContent = "Hệ thống Sẵn sàng";
            overallStatusBadge.className = "tag tag-tls";
        } else {
            dotTunnel.className = "dot dot-off";
            tunnelState.textContent = "Đang kết nối...";
            tunnelState.style.color = "var(--accent-amber)";
            overallStatusBadge.textContent = "Đang kết nối Tunnel...";
            overallStatusBadge.className = "tag tag-notls";
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

// ── Authentication Lifecycle ──────────────────────────────
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

// ── Event Handlers Initialization ─────────────────────────
function initEventListeners() {
    // Password visibility toggle
    togglePasswordBtn.addEventListener("click", () => {
        if (passwordInput.type === "password") {
            passwordInput.type = "text";
            togglePasswordBtn.innerHTML = ICONS.eyeOff;
        } else {
            passwordInput.type = "password";
            togglePasswordBtn.innerHTML = ICONS.eye;
        }
    });

    // Login Submission
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
            loginSubmitBtn.textContent = "Đăng nhập Hệ thống";
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

    // Top Action: Manual Refresh
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

    // Zoom QR Preview
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

    // Log Filtering Controls
    logSearch.addEventListener("input", applyLogFilters);
    logFilterSource.addEventListener("change", applyLogFilters);
    logFilterType.addEventListener("change", applyLogFilters);

    btnToggleScroll.addEventListener("click", () => {
        isAutoScroll = !isAutoScroll;
        btnToggleScroll.innerHTML = (isAutoScroll ? ICONS.chevronDown : ICONS.pause) + (isAutoScroll ? " Auto-scroll" : " Đã dừng");
        btnToggleScroll.className = isAutoScroll ? "btn btn-sm btn-secondary" : "btn btn-sm btn-danger";
    });

    btnClearLogs.addEventListener("click", () => {
        logContainer.innerHTML = `<div class="log-placeholder">Đã xóa nhật ký hiển thị.</div>`;
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

    // Window resize handler for sparkline redraw
    window.addEventListener("resize", () => {
        if (vlessData) {
            fetchStats();
        }
    });
}

// ── Application Bootstrapping ─────────────────────────────
document.addEventListener("DOMContentLoaded", () => {
    initEventListeners();
    checkAuthStatus();
});
