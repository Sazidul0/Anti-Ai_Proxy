import os
import json
import requests
from flask import request, jsonify
from CTFd.plugins import register_plugin_assets_directory
from CTFd.utils.decorators import authed_only
from CTFd.utils.user import get_current_user

# ── Configuration ──

PROXY_API_URL = os.environ.get("PROXY_API_URL", "http://proxy:8081")
PROXY_API_SECRET = os.environ.get("PROXY_API_SECRET", "shared-secret-change-me")


def load(app):
    """CTFd plugin entry point."""
    app.db.create_all()
    register_plugin_assets_directory(app, base_path="/plugins/anti_ai_proxy/assets/")

    # ── Hook into flag submission ──
    @app.before_request
    def check_proxy_connection():
        """Verify user proxy connection before flag submissions."""
        # Only intercept POST to /api/v1/challenges/attempt
        if request.path != "/api/v1/challenges/attempt" or request.method != "POST":
            return None

        user = get_current_user()
        if not user:
            return None

        user_id = user.id

        try:
            # Check if user has active proxy session
            resp = requests.get(
                f"{PROXY_API_URL}/api/sessions/{user_id}/active",
                headers={"X-API-Secret": PROXY_API_SECRET},
                timeout=3,
            )

            if resp.status_code != 200:
                return jsonify({
                    "success": False,
                    "data": {
                        "status": "incorrect",
                        "message": "⚠️ Proxy verification failed. Please ensure you are connected to the Anti-AI proxy."
                    }
                }), 403

            data = resp.json()
            is_active = data.get("active", False)
            session_token = data.get("session_token", "")

            if not is_active:
                # Record the disconnected submission
                _record_submission(user_id, 0, _get_challenge_id(), False)

                return jsonify({
                    "success": False,
                    "data": {
                        "status": "incorrect",
                        "message": "🚫 You must be connected to the Anti-AI proxy to submit flags. "
                                   "Please configure your browser proxy settings and reconnect."
                    }
                }), 403

            # Record successful proxy-connected submission
            challenge_id = _get_challenge_id()
            _record_submission(user_id, 0, challenge_id, True)

        except requests.exceptions.RequestException as e:
            # If proxy API is unreachable, log but allow submission
            # (fail open to avoid blocking the CTF if proxy is down)
            app.logger.error(f"Anti-AI Proxy check failed: {e}")
            return None

        return None

    # ── Admin API endpoints ──
    @app.route("/plugins/anti_ai_proxy/status", methods=["GET"])
    @authed_only
    def proxy_status():
        """Check proxy service status."""
        try:
            resp = requests.get(
                f"{PROXY_API_URL}/api/health",
                timeout=3,
            )
            return jsonify({
                "proxy_online": resp.status_code == 200,
                "proxy_data": resp.json() if resp.status_code == 200 else None,
            })
        except requests.exceptions.RequestException:
            return jsonify({"proxy_online": False, "proxy_data": None})

    @app.route("/plugins/anti_ai_proxy/user/<int:user_id>/check", methods=["GET"])
    @authed_only
    def check_user_proxy(user_id):
        """Check if a specific user is connected to the proxy."""
        try:
            resp = requests.get(
                f"{PROXY_API_URL}/api/sessions/{user_id}/active",
                headers={"X-API-Secret": PROXY_API_SECRET},
                timeout=3,
            )
            return jsonify(resp.json())
        except requests.exceptions.RequestException as e:
            return jsonify({"error": str(e)}), 500

    # ── Settings page ──
    from CTFd.plugins import register_admin_plugin_menu_bar

    register_admin_plugin_menu_bar(
        "Anti-AI Proxy",
        "/plugins/anti_ai_proxy/settings"
    )

    @app.route("/plugins/anti_ai_proxy/settings", methods=["GET", "POST"])
    @authed_only
    def proxy_settings():
        """Plugin settings page."""
        global PROXY_API_URL, PROXY_API_SECRET

        if request.method == "POST":
            PROXY_API_URL = request.form.get("proxy_api_url", PROXY_API_URL)
            PROXY_API_SECRET = request.form.get("proxy_api_secret", PROXY_API_SECRET)
            return jsonify({"success": True, "message": "Settings updated"})

        from flask import render_template_string
        return render_template_string(
            SETTINGS_TEMPLATE,
            proxy_api_url=PROXY_API_URL,
            proxy_api_secret=PROXY_API_SECRET,
        )


def _get_challenge_id():
    """Extract challenge ID from the submission request."""
    try:
        data = request.get_json()
        return data.get("challenge_id", 0) if data else 0
    except Exception:
        return 0


def _record_submission(user_id, session_id, challenge_id, proxy_connected):
    """Record flag submission metadata to the proxy backend."""
    try:
        requests.post(
            f"{PROXY_API_URL}/api/flag-submissions",
            headers={
                "X-API-Secret": PROXY_API_SECRET,
                "Content-Type": "application/json",
            },
            json={
                "user_id": user_id,
                "session_id": session_id,
                "challenge_id": challenge_id,
                "proxy_connected": proxy_connected,
            },
            timeout=3,
        )
    except requests.exceptions.RequestException:
        pass


SETTINGS_TEMPLATE = """
<!DOCTYPE html>
<html>
<head>
    <title>Anti-AI Proxy Settings</title>
    <link rel="stylesheet" href="/themes/core/static/css/style.css">
</head>
<body>
    <div class="container">
        <h1>🛡️ Anti-AI Proxy Gateway Settings</h1>
        <hr>
        <form method="POST" class="form-horizontal">
            <div class="form-group">
                <label for="proxy_api_url"><strong>Proxy API URL</strong></label>
                <input type="text" class="form-control" id="proxy_api_url"
                       name="proxy_api_url" value="{{ proxy_api_url }}"
                       placeholder="http://proxy:8081">
                <small class="form-text text-muted">
                    Internal URL of the Anti-AI Proxy API server.
                </small>
            </div>
            <br>
            <div class="form-group">
                <label for="proxy_api_secret"><strong>API Secret</strong></label>
                <input type="password" class="form-control" id="proxy_api_secret"
                       name="proxy_api_secret" value="{{ proxy_api_secret }}"
                       placeholder="shared-secret">
                <small class="form-text text-muted">
                    Shared secret for authenticating with the proxy API.
                </small>
            </div>
            <br>
            <button type="submit" class="btn btn-primary">Save Settings</button>
        </form>
        <hr>
        <h3>Status</h3>
        <div id="status-container">
            <p>Checking proxy status...</p>
        </div>
        <script>
            fetch('/plugins/anti_ai_proxy/status')
                .then(r => r.json())
                .then(data => {
                    const el = document.getElementById('status-container');
                    if (data.proxy_online) {
                        el.innerHTML = '<div class="alert alert-success">✅ Proxy is ONLINE</div>';
                    } else {
                        el.innerHTML = '<div class="alert alert-danger">❌ Proxy is OFFLINE</div>';
                    }
                })
                .catch(() => {
                    document.getElementById('status-container').innerHTML =
                        '<div class="alert alert-warning">⚠️ Could not reach plugin endpoint</div>';
                });
        </script>
    </div>
</body>
</html>
"""
