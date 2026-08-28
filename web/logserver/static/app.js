const logContainer = document.getElementById("log-container");
let lastLogId = 0;

// Builds one log entry as real DOM nodes (not innerHTML/insertAdjacentHTML
// string interpolation like the original Python template used) — log text
// ultimately originates from xray-core/cloudflared subprocess output, which
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
