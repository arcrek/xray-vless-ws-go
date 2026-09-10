# Phase 3: Dashboard UI Overhaul for Subscription URL & QR

## Context Links
- Plan index: [plan.md](./plan.md)
- Embedded static assets: `web/logserver/static/index.html`, `app.js`, `style.css`
- Logserver go:embed: `web/logserver/embed.go`

## Overview
- **Priority**: P1 (User Facing Deliverable)
- **Status**: Completed
- **Description**: Overhaul the subscription section of the embedded dashboard. Replace the raw Base64 QR code rendering with the public Subscription URL. Because a URL is short (typically 30–60 characters) compared to a multiline Base64 string (2,000+ characters), the generated QR code will have drastically lower dot density, allowing instant scanning by any mobile camera or proxy application. Provide clear UI components to copy the Subscription URL as the primary action, with the raw Base64 string retained as a secondary option for offline imports.

## Key Insights
1. **QR Density Problem**:
   - Currently, `QRCode.renderCanvas(data.base64_config, subQrCanvas)` encodes the entire Base64 string of all nodes. On mobile screens or lower-resolution cameras, high-density QR codes frequently fail to scan.
   - Encoding the Subscription URL (`https://vless.<DOMAIN>/sub` or `http://<HOST>:<PORT>/sub`) reduces QR version and complexity by over 80%, guaranteeing fast and reliable recognition.
2. **Client Synchronization**:
   - V2Ray client apps (v2rayNG, Shadowrocket, Sing-box, V2RayN) identify HTTP/HTTPS URLs as subscription endpoints that can be automatically refreshed periodically. Passing a raw Base64 string bypasses auto-update capability.
3. **Dual Copy Options**:
   - Primary action: "Sao chép URL Subscription" (for auto-updating client subscriptions).
   - Secondary action: "Sao chép chuỗi Base64 (Offline)" (for manual offline import).

## UI / UX Design

### Subscription Card Layout

```
+-------------------------------------------------------------+
| [icon] Mã QR Đăng ký (Subscription URL)                     |
|                                                             |
|                +-------------------------+                  |
|                |                         |                  |
|                |    [COMPACT QR CODE]    |  <-- Click to    |
|                |    (Subscription URL)   |      zoom modal  |
|                |                         |                  |
|                +-------------------------+                  |
|                                                             |
|  +------------------------------------------------+ +----+  |
|  | https://vless.example.com/sub                  | |Copy|  |
|  +------------------------------------------------+ +----+  |
|                                                             |
|  Quét mã QR hoặc sao chép URL Subscription để tự động       |
|  đồng bộ cấu hình trên V2RayN, v2rayNG, Shadowrocket.       |
|                                                             |
|  [  Sao chép URL Subscription  ]  (Primary Blue Button)     |
|  [  Sao chép Base64 (Offline)  ]  (Secondary Outline Button)|
+-------------------------------------------------------------+
```

## Related Code Files

| File Path | Action | Description |
|---|---|---|
| `web/logserver/static/index.html` | Modify | Update Subscription card markup, add URL input, update buttons and tooltips |
| `web/logserver/static/app.js` | Modify | Render QR with `subUrl`, handle URL copy, update modal zoom, update download/copy filename references |
| `web/logserver/static/style.css` | Modify | Add styling for `.sub-url-box`, `.sub-url-input`, and button spacing |

## Implementation Steps

1. **Update `web/logserver/static/index.html`**:
   - In Card 2 Header:
     - Change `#btn-copy-raw-config` title to `"Sao chép toàn bộ nội dung file vless_info.config"`.
     - Change `#btn-download-config` title to `"Tải xuống file vless_info.config"`.
   - In `.sub-card`:
     - Update title to: `<span class="sub-card-title">Mã QR Đăng ký (Subscription URL)</span>`.
     - Under `#qr-preview-box`, add the URL display box:
       ```html
       <div class="sub-url-box">
           <input type="text" id="sub-url-input" class="sub-url-input" readonly spellcheck="false" placeholder="Đang tạo Subscription URL...">
           <button id="btn-copy-sub-url-input" class="btn btn-icon" title="Sao chép URL" aria-label="Sao chép URL">
               <svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>
           </button>
       </div>
       ```
     - Update `.sub-info-text` to: `"Quét mã QR hoặc sao chép URL Subscription để nhập và tự động đồng bộ cấu hình trên V2RayN, v2rayNG, Shadowrocket, Sing-box."`
     - Add primary button:
       ```html
       <button id="btn-copy-sub-url" class="btn btn-primary btn-sm btn-full">
           <svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
           Sao chép URL Subscription
       </button>
       ```
     - Retain `#btn-copy-sub-b64` as secondary button:
       ```html
       <button id="btn-copy-sub-b64" class="btn btn-secondary btn-sm btn-full">
           <svg class="icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>
           Sao chép chuỗi Base64 (Offline)
       </button>
       ```

2. **Update `web/logserver/static/style.css`**:
   - Add styles for `.sub-url-box` and `.sub-url-input`:
     ```css
     .sub-url-box {
         display: flex;
         align-items: center;
         width: 100%;
         gap: 6px;
         background: var(--bg-surface);
         border: 1px solid var(--border-color);
         border-radius: var(--radius-sm);
         padding: 4px 6px;
         margin-bottom: 10px;
     }
     .sub-url-input {
         flex: 1;
         background: transparent;
         border: none;
         color: var(--text-primary);
         font-family: var(--font-mono);
         font-size: 11px;
         outline: none;
         text-overflow: ellipsis;
         white-space: nowrap;
         overflow: hidden;
     }
     .sub-card .btn {
         margin-top: 6px;
     }
     ```

3. **Update `web/logserver/static/app.js`**:
   - Cache elements:
     ```javascript
     const subUrlInput = document.getElementById("sub-url-input");
     const btnCopySubUrl = document.getElementById("btn-copy-sub-url");
     const btnCopySubUrlInput = document.getElementById("btn-copy-sub-url-input");
     ```
   - In `renderVlessInfo(data)`:
     ```javascript
     const subUrl = data.worker_sub_url || (window.location.origin + (data.sub_path || "/sub"));
     if (subUrlInput) {
         subUrlInput.value = subUrl;
     }

     // Render Subscription URL QR Code Canvas (Compact & Low Density)
     if (subUrl && window.QRCode) {
         try {
             QRCode.renderCanvas(subUrl, subQrCanvas, {
                 cellSize: 4,
                 margin: 2,
                 dark: "#000000",
                 light: "#ffffff"
             });
         } catch (e) {
             console.error("Lỗi vẽ mã QR Subscription:", e);
         }
     }
     ```
   - Zoom Preview click handler:
     ```javascript
     qrPreviewBox.addEventListener("click", () => {
         if (vlessData) {
             const subUrl = vlessData.worker_sub_url || (window.location.origin + (vlessData.sub_path || "/sub"));
             openQrModal(subUrl, "Mã QR URL Subscription", subUrl);
         }
     });
     ```
   - Copy handlers:
     ```javascript
     const copySubUrl = () => {
         if (vlessData) {
             const subUrl = vlessData.worker_sub_url || (window.location.origin + (vlessData.sub_path || "/sub"));
             copyToClipboard(subUrl, "Đã sao chép URL Subscription!");
         }
     };
     if (btnCopySubUrl) btnCopySubUrl.addEventListener("click", copySubUrl);
     if (btnCopySubUrlInput) btnCopySubUrlInput.addEventListener("click", copySubUrl);
     ```
   - Update download action toast and filename to `vless_info.config`.

## Todo List
- [x] Update `web/logserver/static/index.html` structure with URL input and updated copy buttons
- [x] Add `.sub-url-box` styling to `web/logserver/static/style.css`
- [x] Update `web/logserver/static/app.js` logic to render `subUrl` in `#sub-qr-canvas` and attach copy handlers
- [x] Update modal zoom handling to render `subUrl` QR code
- [x] Update download filename and toast text to `vless_info.config`

## Success Criteria
- [x] The Subscription card displays the generated Subscription URL in a compact, read-only field.
- [x] The QR code canvas renders the Subscription URL directly (sharp, clean, easily scannable).
- [x] Clicking "Sao chép URL Subscription" copies the URL to the clipboard.
- [x] Clicking "Sao chép chuỗi Base64 (Offline)" copies the raw Base64 string.
- [x] Clicking the QR canvas opens the modal with the Subscription URL QR code.
- [x] Downloading config downloads `vless_info.config`.

## Risk Assessment
- **Risk**: User is on `localhost` or private network IP without domain; Subscription URL would be `http://localhost:9999/sub`.
- **Mitigation**: The fallback to `window.location.origin + "/sub"` accurately uses the exact host the user is using to access the dashboard (e.g. `http://192.168.1.50:9999/sub`). When Cloudflare Worker is active, `data.worker_sub_url` provides the universal public HTTPS URL.
