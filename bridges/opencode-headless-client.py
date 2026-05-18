#!/usr/bin/env python3
"""
opencode-headless-client.py
Wrapper script that talks to OpenCode headless server:
1. Creates a new session
2. Sends prompt_async
3. Auto-approves permission requests
4. Polls /session/{id}/message for the assistant response
5. Returns the last assistant message text
"""

import sys
import json
import time
import uuid
import urllib.request
import urllib.error
import threading

BASE_URL = "http://localhost:4096"
DIRECTORY = "/home/lijialun"
TIMEOUT = 120  # seconds
POLL_INTERVAL = 2

def api(method, path, body=None):
    url = BASE_URL + path
    data = json.dumps(body).encode() if body else None
    req = urllib.request.Request(url, data=data, method=method)
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        resp = urllib.request.urlopen(req, timeout=30)
        ct = resp.headers.get("content-type", "")
        raw = resp.read().decode()
        if not raw.strip():
            return None
        if "json" in ct or raw.strip().startswith(("[", "{")):
            return json.loads(raw)
        return raw
    except urllib.error.HTTPError as e:
        body_text = e.read().decode()
        if e.code == 204:
            return None  # async success
        if body_text.strip().startswith("{"):
            try:
                return json.loads(body_text)
            except Exception:
                pass
        return {"error": e.code, "message": body_text[:200]}

def auto_approve_permissions(session_id, stop_event):
    """Background thread: poll for permission requests and auto-approve them."""
    while not stop_event.is_set():
        try:
            perms = api("GET", f"/permission?directory={DIRECTORY}")
            if isinstance(perms, list):
                for p in perms:
                    if p.get("sessionID") == session_id:
                        pid = p.get("id")
                        if pid:
                            api("POST", f"/permission/{pid}/reply?directory={DIRECTORY}", {"reply": "always"})
        except Exception:
            pass
        stop_event.wait(POLL_INTERVAL)

def main():
    if len(sys.argv) < 2:
        print("Usage: opencode-headless-client.py <prompt text>")
        sys.exit(1)

    prompt_text = " ".join(sys.argv[1:])

    # 1. Create session
    session = api("POST", f"/session?directory={DIRECTORY}", {})
    if not session or isinstance(session, dict) and "error" in session:
        print(json.dumps({"error": "Failed to create session", "detail": session}))
        sys.exit(1)

    session_id = session["id"]
    msg_id = f"msg_{uuid.uuid4().hex[:12]}"

    # 2. Start auto-approve thread
    stop_approve = threading.Event()
    approve_thread = threading.Thread(target=auto_approve_permissions, args=(session_id, stop_approve), daemon=True)
    approve_thread.start()

    # 3. Send prompt
    api("POST", f"/session/{session_id}/prompt_async?directory={DIRECTORY}", {
        "messageID": msg_id,
        "model": {"providerID": "zai-coding-plan", "modelID": "glm-5-turbo"},
        "parts": [{"type": "text", "text": prompt_text}]
    })

    # 4. Poll for messages
    start = time.time()
    last_response = ""
    got_assistant = False

    while time.time() - start < TIMEOUT:
        time.sleep(POLL_INTERVAL)
        try:
            messages = api("GET", f"/session/{session_id}/message?directory={DIRECTORY}&limit=20")
        except Exception:
            continue

        if not isinstance(messages, list) or len(messages) < 2:
            continue

        # Look for assistant message with text
        for msg in reversed(messages):
            info = msg.get("info", {})
            if info.get("role") == "assistant":
                parts = msg.get("parts", [])
                for p in parts:
                    if p.get("type") == "text" and p.get("text", "").strip():
                        last_response = p["text"].strip()
                        got_assistant = True

        # Check if all tool calls are done (no running tools)
        all_done = True
        for msg in messages:
            if msg.get("info", {}).get("role") == "assistant":
                for p in msg.get("parts", []):
                    if p.get("type") == "tool":
                        state = p.get("state", {})
                        if state.get("status") == "running":
                            all_done = False

        if got_assistant and all_done:
            stop_approve.set()
            print(last_response)
            sys.exit(0)

    # Timeout
    stop_approve.set()
    if last_response:
        print(last_response)
    else:
        print(json.dumps({"error": "timeout", "session_id": session_id}))
        sys.exit(1)

if __name__ == "__main__":
    main()
