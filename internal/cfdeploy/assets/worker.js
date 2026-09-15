/*
Cloudflare Worker script to dynamically configure
the target host and path for WebSocket proxying.

This is the Go repo's own embedded copy (see internal/cfdeploy) — no file
outside this repo is read at runtime. The password line's literal is a
template marker (__WORKER_PASSWORD__), substituted by deploy.go before
upload; do not hand-edit it into a real password here.

Created by: HuskyDG
*/

// initial global variables to store the target host, path, transport, and subscription
let GLOBAL_TARGET_HOST = "";
let GLOBAL_TARGET_PATH = "";
let GLOBAL_ENTRY_PATH = "";
let GLOBAL_TARGET_TRANSPORT = "websocket";
let GLOBAL_SUB = "";
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    // ==========================================
    // DYNAMIC CONFIGURATION API (POST /setapi?password=...)
    // ==========================================
    if (url.pathname === "/setapi") {
      if (request.method !== "GET" && request.method !== "POST") {
        return new Response("Method Not Allowed", { status: 405 });
      }

      const password = url.searchParams.get("password");
      if (password !== "__WORKER_PASSWORD__") {
        return new Response("Unauthorized", { status: 403 });
      }

      let wshost = "";
      let wspath = "";
      let entrypath = "";
      let transport = "";
      try {
        if (request.method == "POST") {
          const jsonBody = await request.json();
          wshost = jsonBody.wshost;
          wspath = jsonBody.wspath;
          entrypath = jsonBody.entrypath;
          transport = jsonBody.transport;
          const payloads = jsonBody.payloads;
          if (Array.isArray(payloads) && payloads.length > 0) {
            try {
              const subContent = btoa(unescape(encodeURIComponent(payloads.join("\n"))));
              GLOBAL_SUB = subContent;
              if (env.KV_CONFIG) {
                await env.KV_CONFIG.put("SUBSCRIPTION", subContent);
              }
            } catch (_) {}
          }
        } else {
          wshost = url.searchParams.get("wshost");
          wspath = url.searchParams.get("wspath");
          entrypath = url.searchParams.get("entrypath");
          transport = url.searchParams.get("transport");
        }
        if (!entrypath) entrypath = wspath;
        if (!wshost || !wspath) {
          return new Response("Bad Request", { status: 400 });
        }

        // 1. Update the global variables in RAM for immediate use
        GLOBAL_TARGET_HOST = wshost;
        GLOBAL_TARGET_PATH = wspath;
        GLOBAL_ENTRY_PATH = entrypath;
        if (transport) {
          GLOBAL_TARGET_TRANSPORT = transport;
        }

        // 2. Also save to KV for persistence across worker restarts
        if (env.KV_CONFIG) {
          await env.KV_CONFIG.put("TARGET_HOST", wshost);
          await env.KV_CONFIG.put("TARGET_PATH", wspath);
          await env.KV_CONFIG.put("ENTRY_PATH", entrypath);
          if (transport) {
            await env.KV_CONFIG.put("TARGET_TRANSPORT", transport);
          }
        }
        return new Response(JSON.stringify({
          status: "success",
          message: "Worker configured successfully via Go auto-deploy webhook!",
          saved_host: wshost,
          saved_path: wspath,
          entry_path: entrypath,
          saved_transport: GLOBAL_TARGET_TRANSPORT,
          kv_setup: (env.KV_CONFIG)? true : false
        }), {
          status: 200,
          headers: { "Content-Type": "application/json" }
        });

      } catch (error) {
        return new Response("Bad Request", { status: 400 });
      }
    }

    // ==========================================
    // PUBLIC SUBSCRIPTION ENDPOINT (/sub, /subscription)
    // ==========================================
    if (url.pathname === "/sub" || url.pathname === "/subscription") {
      let sub = GLOBAL_SUB;
      if (!sub && env.KV_CONFIG) {
        sub = await env.KV_CONFIG.get("SUBSCRIPTION");
        if (sub) GLOBAL_SUB = sub;
      }
      if (!sub) {
        return new Response("Subscription not ready", { status: 503 });
      }
      let body = sub;
      if (url.searchParams.get("raw") === "1" || url.searchParams.get("raw") === "true") {
        try {
          body = decodeURIComponent(escape(atob(sub)));
        } catch (_) {
          try {
            body = atob(sub);
          } catch (_) {}
        }
      }
      return new Response(body, {
        status: 200,
        headers: {
          "Content-Type": "text/plain; charset=utf-8",
          "Cache-Control": "no-cache, no-store, must-revalidate",
          "Access-Control-Allow-Origin": "*",
          "Profile-Update-Interval": "24",
          "Content-Disposition": 'inline; filename="vless_subscription.txt"',
          "Subscription-Userinfo": "upload=0; download=0; total=1073741824000; expire=0"
        }
      });
    }

    // If the request is not to /setapi or /sub, proceed to handle it as a proxy request
    if (env.KV_CONFIG) {
      if (!GLOBAL_TARGET_HOST || !GLOBAL_TARGET_PATH) {
          // If the global variables are empty, try to load from KV (if KV is set up)
          const kvHost = await env.KV_CONFIG.get("TARGET_HOST");
          const kvPath = await env.KV_CONFIG.get("TARGET_PATH");
          const kvEntryPath = await env.KV_CONFIG.get("ENTRY_PATH");
          const kvTransport = await env.KV_CONFIG.get("TARGET_TRANSPORT");

          if (kvHost && kvPath) {
            GLOBAL_TARGET_HOST = kvHost;
            GLOBAL_TARGET_PATH = kvPath;
            GLOBAL_ENTRY_PATH = kvEntryPath;
            if (kvTransport) GLOBAL_TARGET_TRANSPORT = kvTransport;
          }
      }
    }
    // If still empty, return 503
    const targetHost = (GLOBAL_TARGET_HOST.startsWith("https://") || GLOBAL_TARGET_HOST.startsWith("http://"))?
      GLOBAL_TARGET_HOST : "https://" + GLOBAL_TARGET_HOST;
    let targetPath = GLOBAL_TARGET_PATH;

    // If targetHost or targetPath is not set, return 503 Service Unavailable
    if (!targetHost || !targetPath) {
      return new Response("Service Unavailable: No configuration found.", { status: 503 });
    }

    if (!targetPath.startsWith("/")) targetPath = "/" + targetPath;

    let entryPath = (GLOBAL_ENTRY_PATH)? GLOBAL_ENTRY_PATH : targetPath;
    if (!entryPath.startsWith("/")) entryPath = "/" + entryPath;

    // Normalize paths: strip trailing slashes (except root "/") to ensure clean routing
    const normEntry = (entryPath.length > 1 && entryPath.endsWith("/")) ? entryPath.slice(0, -1) : entryPath;
    const normTarget = (targetPath.length > 1 && targetPath.endsWith("/")) ? targetPath.slice(0, -1) : targetPath;

    // ==========================================
    // FORWARD PROXY LOGIC (WebSocket & XHTTP)
    // ==========================================

    // 1. WebSocket Upgrade logic
    const isWebSocket = request.headers.get("Upgrade") === "websocket";
    if (isWebSocket && (url.pathname === normEntry || (normEntry === "/" && url.pathname === "/"))) {
      const cleanPath = (normTarget === "/" ? "" : normTarget).replace(/\/+/g, "/");
      const targetUrl = new URL((cleanPath || "/") + url.search, targetHost);

      const newHeaders = new Headers(request.headers);
      newHeaders.set("Host", targetUrl.host);

      try {
        return await fetch(targetUrl.toString(), {
          method: "GET",
          headers: newHeaders
        });
      } catch (_) {
        return new Response("Bad Gateway: Origin unreachable", { status: 502 });
      }
    }

    // 2. XHTTP (SplitHTTP) session logic
    // Matches entryPath exactly or subpaths under entryPath/ (for session IDs)
    const pathMatchesXhttp = normEntry === "/"
      ? true
      : (url.pathname === normEntry || url.pathname.startsWith(normEntry + "/"));

    if (pathMatchesXhttp) {
      const suffix = normEntry === "/" ? url.pathname : url.pathname.slice(normEntry.length);
      const cleanPath = (normTarget === "/" ? suffix : `${normTarget}${suffix}`).replace(/\/+/g, "/");
      const targetUrl = new URL((cleanPath || "/") + url.search, targetHost);

      const newHeaders = new Headers(request.headers);
      newHeaders.set("Host", targetUrl.host);

      const isGetOrHead = request.method === "GET" || request.method === "HEAD";
      try {
        return await fetch(targetUrl.toString(), {
          method: request.method,
          headers: newHeaders,
          body: isGetOrHead ? undefined : request.body,
          duplex: isGetOrHead ? undefined : "half"
        });
      } catch (_) {
        return new Response("Bad Gateway: Origin unreachable", { status: 502 });
      }
    }

    return new Response("Not Found", { status: 404 });
  }
};
