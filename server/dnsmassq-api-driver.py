#!/usr/bin/env python3

import re
import json
import logging
import subprocess
import os
from threading import Lock
from flask import Flask, request, jsonify, abort

API_KEY = os.getenv("API_KEY", "6208de06706682ba75ffe49a2b458af0")
LOG_FILE = "/var/log/dns_api.log"

RE_DOMAIN = re.compile(r"^(?:[a-zA-Z0-9-]+\.)*[a-zA-Z0-9-]+$")
RE_IP = re.compile(r"^(?:\d{1,3}\.){3}\d{1,3}$")

app = Flask(__name__)
lock = Lock()

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s", handlers=[logging.FileHandler(LOG_FILE), logging.StreamHandler()])

def check_auth():
    key = request.headers.get("X-API-Key")
    if key != API_KEY:
        logging.warning("Unauthorized from %s", request.remote_addr)
        abort(401)

def validate_domain(domain):
    return bool(RE_DOMAIN.fullmatch(domain))

def validate_ip(ip):
    if not RE_IP.fullmatch(ip):
        return False
    parts = ip.split(".")
    return all(0 <= int(p) <= 255 for p in parts)

def run_cmd(args):
    try:
        r = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        return (r.returncode, r.stdout.strip(), r.stderr.strip())
    except Exception as e:
        return (255, "", str(e))

def get_records():
    rc, out, err = run_cmd(["uci", "show", "dhcp"])
    if rc != 0:
        return None, err
    
    records = []
    for line in out.splitlines():
        if ".address=" not in line:
            continue
        try:
            k, v = line.split("=", 1)
            v = v.strip().strip("'").strip('"')
            
            entries = []
            if "' '" in v:
                parts = v.split("' '")
                for part in parts:
                    part = part.strip().strip("'").strip('"')
                    if part:
                        entries.append(part)
            else:
                entries = [v]
            
            for entry in entries:
                if entry.startswith("/"):
                    entry = entry[1:]
                if entry.count("/") == 1:
                    domain, ip = entry.split("/", 1)
                    if domain and ip and "." in domain:
                        records.append({"domain": domain.strip(), "ip": ip.strip()})
                        
        except Exception as e:
            logging.warning(f"Failed to parse line: {line} - {e}")
            continue
    
    return records, None

@app.before_request
def auth_check():
    if request.path != "/health":
        check_auth()

@app.route("/health")
def health():
    return {"status": "ok"}

@app.route("/dns", methods=["GET"])
def list_dns():
    records, err = get_records()
    if records is None:
        return {"error": err}, 500
    return {"records": records}

@app.route("/dns", methods=["POST"])
def add_dns():
    data = request.get_json(force=True)
    domain = data.get("domain", "").strip()
    ip = data.get("ip", "").strip()

    if not domain or not ip:
        return {"error": "domain and ip required"}, 400
    if not validate_domain(domain) or not validate_ip(ip):
        return {"error": "invalid format"}, 400

    entry = f"/{domain}/{ip}"

    with lock:
        records, _ = get_records()
        if any(r["domain"] == domain and r["ip"] == ip for r in records):
            return {"status": "exists"}

        rc, _, err = run_cmd(["uci", "add_list", f"dhcp.@dnsmasq[0].address={entry}"])
        if rc != 0:
            return {"error": "add failed", "detail": err}, 500

        run_cmd(["uci", "commit", "dhcp"])
        run_cmd(["/etc/init.d/dnsmasq", "reload"])

    logging.info(f"Added {domain} -> {ip}")
    return {"status": "added", "domain": domain, "ip": ip}

@app.route("/dns", methods=["PUT"])
def update_dns():
    data = request.get_json(force=True)
    domain = data.get("domain", "").strip()
    ip = data.get("ip", "").strip()
    new_ip = data.get("new_ip", "").strip()

    if not domain or not new_ip:
        return {"error": "domain and new_ip required"}, 400
    if not validate_domain(domain) or not validate_ip(new_ip):
        return {"error": "invalid format"}, 400

    with lock:
        records, _ = get_records()
        matches = [r for r in records if r["domain"] == domain and (not ip or r["ip"] == ip)]
        if not matches:
            return {"error": "not found"}, 404

        for r in matches:
            entry = f"/{r['domain']}/{r['ip']}"
            run_cmd(["uci", "del_list", f"dhcp.@dnsmasq[0].address={entry}"])

        new_entry = f"/{domain}/{new_ip}"
        run_cmd(["uci", "add_list", f"dhcp.@dnsmasq[0].address={new_entry}"])
        run_cmd(["uci", "commit", "dhcp"])
        run_cmd(["/etc/init.d/dnsmasq", "reload"])

    logging.info(f"Updated {domain} -> {new_ip}")
    return {"status": "updated", "domain": domain, "new_ip": new_ip}

@app.route("/dns", methods=["DELETE"])
def delete_dns():
    data = request.get_json(force=True)
    domain = data.get("domain", "").strip()
    ip = data.get("ip", "").strip()

    if not domain:
        return {"error": "domain required"}, 400
    if not validate_domain(domain):
        return {"error": "invalid domain"}, 400

    with lock:
        records, _ = get_records()
        matches = [r for r in records if r["domain"] == domain and (not ip or r["ip"] == ip)]
        if not matches:
            return {"error": "not found"}, 404

        for r in matches:
            entry = f"/{r['domain']}/{r['ip']}"
            run_cmd(["uci", "del_list", f"dhcp.@dnsmasq[0].address={entry}"])

        run_cmd(["uci", "commit", "dhcp"])
        run_cmd(["/etc/init.d/dnsmasq", "reload"])

    logging.info(f"Deleted {domain}")
    return {"status": "deleted", "domain": domain}

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=18081)