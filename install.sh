#!/usr/bin/env bash
# xray-vless-ws-go quickstart installer.
#
# What it does:
#   1. Asks for the .env values interactively (Enter keeps the default).
#   2. Downloads the matching prebuilt binary + SHA256SUMS from this repo's
#      latest GitHub Release and verifies the checksum.
#   3. Runs the binary in the foreground.
#
# The repo is public, so downloading is a plain, unauthenticated `wget`
# against the public release-asset URLs — no token or `gh` login needed.
#
# Usage:
#   ./install.sh                # interactive, installs into ./xrayws-run
#   INSTALL_DIR=/opt/xrayws ./install.sh
#   RELEASE_TAG=v1.0.0 ./install.sh   # pin a specific release instead of latest
#
# Quickstart one-liner downloads to a temp file and runs that, on purpose —
# `wget -qO- ... | bash` would stream this script's own bytes in on fd 0,
# and the `exec </dev/tty` below would then repoint bash's *script-reading*
# fd away from that pipe mid-parse, truncating the run right after this
# block with no output and no error. See README's Quickstart section.

set -euo pipefail

# Belt-and-braces stdin fallback: if something upstream still redirects our
# stdin away from a terminal (e.g. invoked from another script, or run
# under a supervisor with stdin closed), rebind to the controlling tty so
# the interactive prompts still work instead of hitting instant EOF.
if [ ! -t 0 ] && [ -r /dev/tty ]; then
  # `-r /dev/tty` only checks permission bits; the node can still fail to
  # open (ENXIO) when the process has no controlling terminal at all (CI,
  # `docker run` without `-t`, some sandboxes). Probe in a subshell first so
  # a failed open doesn't kill the whole script under `set -e`, and doesn't
  # leave a scary "No such device or address" on stderr for a case we
  # already expect and handle.
  if (exec </dev/tty) 2>/dev/null; then
    exec </dev/tty
  fi
fi

REPO="${REPO:-arcrek/xray-vless-ws-go}"
INSTALL_DIR="${INSTALL_DIR:-$PWD/xrayws-run}"
RELEASE_TAG="${RELEASE_TAG:-latest}"

# ---------------------------------------------------------------------------
# 1. Collect .env values
# ---------------------------------------------------------------------------

ask() {
  # ask <var_name> <prompt> <default>
  local __name="$1" __prompt="$2" __default="$3" __reply
  read -r -p "$__prompt [$__default]: " __reply || true
  printf -v "$__name" '%s' "${__reply:-$__default}"
}

echo "== xray-vless-ws-go quickstart =="
echo "Enter to keep the shown default."
echo

ask PORT             "Local listen address (host:port)" "127.0.0.1:8888"
ask XRAY_UUID        "VLESS client UUID (blank = auto-generate on first run)" ""
ask FAKE_SNI         "Fake SNI list (name#label,name#label,...)" "api24-normal-alisg.tiktokv.com#Tiktok"
ask WS_PATH          "WebSocket path" "/tiktok4g"
ask TRANSPORT        "Transport" "websocket"
ask WEBHOOK_URL      "Webhook URL for link updates (blank = disabled)" ""

echo
echo "Dashboard (http://<host>:9999/ by default): required Basic-Auth password —"
echo "the dashboard shows live traffic stats and the tunnel hostname, so it"
echo "refuses to start with no password set."
default_log_password="$(head -c18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c24)"
ask LOG_PASSWORD      "Dashboard password" "$default_log_password"

echo
echo "Tunnel mode: leave both blank for a zero-setup quick tunnel (dev/test)."
echo "For production, set a named Cloudflare Tunnel token instead."
ask TUNNEL_TOKEN     "Cloudflare named tunnel token (blank = quick tunnel)" ""
if [ -n "$TUNNEL_TOKEN" ]; then
  ask WS_HOST        "Public hostname configured for that tunnel" ""
else
  WS_HOST="trycloudflare.com"
fi

echo
echo "Cloudflare Worker auto-deploy (optional): provisions the Worker bridge"
echo "+ KV binding + custom domain route via the Cloudflare REST API instead"
echo "of doing it by hand in the dashboard. Leave blank to skip (WEBHOOK_URL"
echo "above is then used as-is, unchanged behavior)."
echo "Token needs: Workers Scripts:Edit, Workers KV Storage:Edit,"
echo "Workers Routes:Edit (Zone), Zone:Read (Zone) — scoped to DOMAIN's zone."
ask CLOUDFLARE_API_TOKEN "Cloudflare API token (blank = skip auto-deploy)" ""
if [ -n "$CLOUDFLARE_API_TOKEN" ]; then
  echo "  (zone apex only, no 'vless.' prefix — the app adds that itself,"
  echo "   e.g. example.com becomes vless.example.com)"
  ask DOMAIN                "Zone apex already on that account (e.g. example.com)" ""
  ask CLOUDFLARE_ACCOUNT_ID "Account ID (blank = auto-resolve via GET /accounts)" ""
  ask WORKER_PASSWORD       "Worker /setapi password (blank = random on first run)" ""
else
  DOMAIN=""
  CLOUDFLARE_ACCOUNT_ID=""
  WORKER_PASSWORD=""
fi

mkdir -p "$INSTALL_DIR"
ENV_FILE="$INSTALL_DIR/.env"

cat > "$ENV_FILE" <<EOF
PORT=$PORT
XRAY_UUID=$XRAY_UUID
FAKE_SNI=$FAKE_SNI
WS_PATH=$WS_PATH
WS_HOST=$WS_HOST
TRANSPORT=$TRANSPORT
WEBHOOK_URL=$WEBHOOK_URL
LOG_PASSWORD=$LOG_PASSWORD
TUNNEL_TOKEN=$TUNNEL_TOKEN
CLOUDFLARE_API_TOKEN=$CLOUDFLARE_API_TOKEN
CLOUDFLARE_ACCOUNT_ID=$CLOUDFLARE_ACCOUNT_ID
DOMAIN=$DOMAIN
WORKER_PASSWORD=$WORKER_PASSWORD
EOF
chmod 600 "$ENV_FILE"

echo
echo "Wrote $ENV_FILE"

# ---------------------------------------------------------------------------
# 2. Detect platform and download the matching release binary
# ---------------------------------------------------------------------------

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Linux)  goos=linux ;;
  Darwin) goos=darwin ;;
  *) echo "Unsupported OS: $os (use the Windows binary manually for Windows)"; exit 1 ;;
esac

case "$arch" in
  x86_64|amd64) goarch=amd64 ;;
  aarch64|arm64) goarch=arm64 ;;
  *) echo "Unsupported architecture: $arch"; exit 1 ;;
esac

asset="xrayws-${goos}-${goarch}"
bin_path="$INSTALL_DIR/xrayws"

echo
echo "Target asset: $asset (release: $RELEASE_TAG)"

if [ "$RELEASE_TAG" = "latest" ]; then
  base_url="https://github.com/$REPO/releases/latest/download"
else
  base_url="https://github.com/$REPO/releases/download/$RELEASE_TAG"
fi

for name in "$asset" "SHA256SUMS"; do
  wget -qO "$INSTALL_DIR/$name" "$base_url/$name"
done

mv "$INSTALL_DIR/$asset" "$bin_path"
chmod +x "$bin_path"

# ---------------------------------------------------------------------------
# 3. Verify checksum
# ---------------------------------------------------------------------------

sums_file="$INSTALL_DIR/SHA256SUMS"
if [ -f "$sums_file" ]; then
  expected="$(grep " $asset\$" "$sums_file" | awk '{print $1}')"
  if [ -z "$expected" ]; then
    echo "WARNING: $asset not listed in SHA256SUMS, skipping verification." >&2
  else
    if command -v sha256sum >/dev/null 2>&1; then
      actual="$(sha256sum "$bin_path" | awk '{print $1}')"
    else
      actual="$(shasum -a 256 "$bin_path" | awk '{print $1}')"
    fi
    if [ "$expected" != "$actual" ]; then
      echo "Checksum mismatch for $asset! expected=$expected actual=$actual" >&2
      exit 1
    fi
    echo "Checksum verified OK."
  fi
else
  echo "WARNING: SHA256SUMS not downloaded, skipping verification." >&2
fi

# ---------------------------------------------------------------------------
# 4. Run
# ---------------------------------------------------------------------------

echo
echo "Installed: $bin_path"
echo "Config:    $ENV_FILE"
echo
echo "Starting xrayws..."
cd "$INSTALL_DIR"
exec ./xrayws
