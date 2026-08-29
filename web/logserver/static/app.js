const logContainer = document.getElementById("log-container");
let lastLogId = 0;

// Builds one log entry as real DOM nodes (not innerHTML/insertAdjacentHTML
// string interpolation) — log text ultimately originates from
// xray-core/cloudflared subprocess output, which
// this viewer should not trust as safe markup. textContent keeps every
// character literal, closing that XSS sink while keeping the same visual
// output.
function buildLogEntry(item) {
    const type = (item.type || "INFO").toUpperCase();
    let typeClass = "type-info";
    if (type.includes("ERROR")) typeClass = "type-error";
    if (type.includes("SUCCESS")) typeClass = "type-success";

    const entry = document.createElement("div");
    entry.className = "log-entry";

    const ts = document.createElement("span");
    ts.className = "timestamp";
    ts.textContent = `[${item.time}]`;
    entry.appendChild(ts);

    const typeSpan = document.createElement("span");
    typeSpan.className = typeClass;
    typeSpan.textContent = `[${type}] `;
    entry.appendChild(typeSpan);

    if (typeof item.text === "object") {
        const pre = document.createElement("pre");
        pre.textContent = JSON.stringify(item.text, null, 2);
        entry.appendChild(pre);
    } else {
        entry.appendChild(document.createTextNode(String(item.text)));
    }

    return entry;
}

async function fetchLogs() {
    try {
        const res = await fetch(`/logs?last_id=${lastLogId}`);
        const data = await res.json();
        if (data.new_logs.length > 0) {
            if (logContainer.textContent.includes("Đang chờ")) logContainer.innerHTML = "";
            for (const item of data.new_logs) {
                logContainer.appendChild(buildLogEntry(item));
            }
            lastLogId = data.last_id;
            logContainer.scrollTop = logContainer.scrollHeight;
        }
    } catch (e) {}
}
setInterval(fetchLogs, 1000);

function formatBps(n) {
    if (n < 1024) return `${n} B/s`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB/s`;
    return `${(n / 1024 / 1024).toFixed(1)} MB/s`;
}

function formatUptime(sec) {
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = Math.floor(sec % 60);
    return `${h}h ${m}m ${s}s`;
}

function drawSparkline(history) {
    const canvas = document.getElementById("sparkline");
    const ctx = canvas.getContext("2d");
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    if (history.length < 2) return;

    // x-position by timestamp, not array index — a sampling gap should
    // render as a gap, not a smooth evenly-spaced line hiding an outage.
    let maxVal = 1;
    for (const h of history) maxVal = Math.max(maxVal, h.up_bps, h.down_bps);

    const tMin = history[0].t;
    const tMax = history[history.length - 1].t;
    const tSpan = Math.max(1, tMax - tMin);

    function drawLine(key, color) {
        ctx.beginPath();
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        history.forEach((point, i) => {
            const x = ((point.t - tMin) / tSpan) * canvas.width;
            const y = canvas.height - (point[key] / maxVal) * canvas.height;
            if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        });
        ctx.stroke();
    }
    drawLine("up_bps", "#2ed573");
    drawLine("down_bps", "#007bff");
}

async function fetchStats() {
    try {
        const res = await fetch("/stats");
        const s = await res.json();

        document.getElementById("dot-xray").className = "dot " + (s.xray_up ? "dot-on" : "dot-off");
        document.getElementById("xray-state").textContent = s.xray_up ? "started" : "stopped"; // "started", not "running" — xray_up is not a continuous liveness signal the way tunnel_ready is
        document.getElementById("dot-tunnel").className = "dot " + (s.tunnel_ready ? "dot-on" : "dot-off");
        document.getElementById("tunnel-state").textContent = s.tunnel_ready ? "ready" : "connecting";
        document.getElementById("ready-connections").textContent = s.ready_connections;
        document.getElementById("hostname").textContent = s.hostname || "—";
        document.getElementById("uptime").textContent = formatUptime(s.uptime_sec);
        document.getElementById("up-bps").textContent = formatBps(s.uplink_bps);
        document.getElementById("down-bps").textContent = formatBps(s.downlink_bps);
        drawSparkline(s.history || []);
    } catch (e) {}
}
fetchStats();
setInterval(fetchStats, 2000);
