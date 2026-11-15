"""End-to-end check of the admin panel's API against a LOCAL Athlix API. Prints no secrets.

Covers: dashboard, user search / detail / suspend / unsuspend, location-identity (player)
verification review, organiser verification reject -> resubmit -> approve, tournament review
(approve, feature, unfeature, remove, reject with refund), reports and moderation (hide,
dismiss), listing orders and fee preview, config write + history, role-based permissions
(a limited admin gets 403), and the audit log.

usage (same admin env vars as smoke_test.py, an admin with super_admin role):
  SMOKE_ADMIN_EMAIL=... SMOKE_ADMIN_PASSWORD=... SMOKE_ADMIN_TOTP_URI='otpauth://...' \
    python3 -I admin_flows_test.py http://127.0.0.1:8080/v1
"""
import base64, hashlib, hmac, json, os, struct, sys, time, urllib.error, urllib.parse, urllib.request
from datetime import datetime, timedelta, timezone

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8080/v1"
if not BASE.startswith(("http://127.0.0.1", "http://localhost")):
    sys.exit("admin flows test runs against a local API only")
RUN = "%06d" % (int(time.time()) % 1000000)
ok_count = 0
# 1x1 JPEG, enough for a location proof upload in local storage.
JPEG = base64.b64decode(
    "/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAP//////////////////////////////////////////////////////////////"
    "////////////////////////wgALCAABAAEBAREA/8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABPxA=")


def call(method, path, body=None, token=None, expect=200, raw_url=None, raw_body=None, headers=None):
    url = raw_url or BASE + path
    data = raw_body if raw_body is not None else (None if body is None else json.dumps(body).encode())
    req = urllib.request.Request(url, data=data, method=method)
    if raw_body is None:
        req.add_header("content-type", "application/json")
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    if token:
        req.add_header("authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req) as r:
            status, raw = r.status, r.read()
    except urllib.error.HTTPError as e:
        status, raw = e.code, e.read()
    try:
        out = json.loads(raw) if raw else {}
    except ValueError:
        out = {"raw": raw[:200].decode(errors="replace")}
    expected = expect if isinstance(expect, tuple) else (expect,)
    if status not in expected:
        print(f"FAIL {method} {path or url}: want {expect} got {status}: {json.dumps(out)[:700]}")
        sys.exit(1)
    return out.get("data", out) if isinstance(out, dict) else out


def items(page):
    return page["items"] if isinstance(page, dict) and "items" in page else page


def check(cond, label, detail=""):
    global ok_count
    if not cond:
        print("FAIL", label, json.dumps(detail, default=str)[:700] if detail != "" else "")
        sys.exit(1)
    ok_count += 1
    print("ok  ", label)


def totp(uri):
    secret = urllib.parse.parse_qs(urllib.parse.urlparse(uri).query)["secret"][0]
    key = base64.b32decode(secret + "=" * (-len(secret) % 8))
    h = hmac.new(key, struct.pack(">Q", int(time.time()) // 30), hashlib.sha1).digest()
    o = h[-1] & 15
    return "%06d" % ((struct.unpack(">I", h[o:o + 4])[0] & 0x7FFFFFFF) % 1000000)


def admin_login(email, password, uri):
    time.sleep(31 - time.time() % 30)  # fresh TOTP window: codes are single-use per window
    return call("POST", "/admin/auth/login", {"email": email, "password": password, "totpCode": totp(uri)})


def onboard(mobile, role, name, district=26):
    r = call("POST", "/auth/otp/request", {"mobile": mobile})
    tok = call("POST", "/auth/otp/verify", {"requestId": r["requestId"], "otp": "123456"})["tokens"]["accessToken"]
    call("POST", "/onboarding/role", {"role": role}, tok)
    me = call("GET", "/me", token=tok)
    if me["pendingPolicies"]:
        call("POST", "/me/policies/accept", {"policyDocumentIds": [p["id"] for p in me["pendingPolicies"]]}, tok)
    call("POST", "/verification/mock/complete", {"outcome": "adult", "nativeDistrictId": district}, tok)
    me = call("POST", "/onboarding/profile", {"displayName": name, "sportId": 1, "city": "Chennai", "state": "Tamil Nadu"}, tok)
    return tok, me


def organiser_submit(tok, org_name):
    v = call("GET", "/organiser/verification", token=tok)
    steps = []
    for s in v["steps"]:
        if s["inputType"] == "checkbox":
            steps.append({"stepId": s["stepId"], "checked": True})
        elif s["inputType"] == "text":
            steps.append({"stepId": s["stepId"], "valueText": org_name})
    if any(s["inputType"] == "document" and s["isRequired"] for s in v["steps"]):
        sys.exit("organiser verification needs a document upload; extend this test")
    call("PUT", "/organiser/verification", {"orgName": org_name, "orgType": "club", "steps": steps}, tok)
    call("POST", "/organiser/verification/submit", {}, tok)


def new_event(tok, title):
    now = datetime.now(timezone.utc)
    iso = lambda d: d.strftime("%Y-%m-%dT%H:%M:%SZ")
    ev = call("POST", "/events", {
        "sportId": 1, "title": title, "description": "Admin flows test event",
        "eventType": "tournament", "participantType": "individual", "competitionLevel": "district",
        "genderCategory": "open", "venueName": "Ground", "addressLine": "Main Road", "city": "Chennai",
        "state": "Tamil Nadu", "pincode": "600001",
        "startsAt": iso(now + timedelta(days=20)), "endsAt": iso(now + timedelta(days=21)),
        "registrationDeadline": iso(now + timedelta(days=10)),
        "entryFeePaise": 20000, "paymentModes": ["cash"], "paymentInstructions": "Pay at the ground",
        "maxSlots": 8, "contactName": "Ravi", "contactPhone": "9876543210", "category": "private",
    }, tok, expect=201)
    order = call("POST", f"/events/{ev['id']}/listing/order", {}, tok)
    call("POST", "/listing/mock/pay", {"orderId": order["orderId"]}, tok)
    return ev["id"], order["orderId"]


# ---------------------------------------------------------------- setup
call("GET", "/health")
pName, oName, rName = f"AdmPlayer{RUN}", f"AdmOrg{RUN}", f"AdmReporter{RUN}"
pTok, player = onboard("6" + RUN + "301", "player", pName, district=4)
oTok, org = onboard("6" + RUN + "302", "organiser", oName)
rTok, reporter = onboard("6" + RUN + "303", "player", rName)
pid, oid = player["id"], org["id"]

login = admin_login(os.environ["SMOKE_ADMIN_EMAIL"], os.environ["SMOKE_ADMIN_PASSWORD"], os.environ["SMOKE_ADMIN_TOTP_URI"])
adm = login["tokens"]["accessToken"]
check("*" in login["admin"]["permissions"] or len(login["admin"]["permissions"]) > 10, "super admin logged in", login["admin"])

# ---------------------------------------------------------------- dashboard
dash = call("GET", "/admin/dashboard", token=adm)
check(all(k in dash for k in ("users", "events", "organisers", "reports", "revenue")) and dash["users"]["total"] >= 3,
      "dashboard returns counts", dash)

# ---------------------------------------------------------------- user management
rows = items(call("GET", "/admin/users?" + urllib.parse.urlencode({"q": pName}), token=adm))
check(any(r["id"] == pid for r in rows), "user search by name finds the player", rows)
row = [r for r in rows if r["id"] == pid][0]
check(row["mobileMasked"].startswith("*") and "301" in row["mobileMasked"], "mobile is masked in the list", row["mobileMasked"])
det = call("GET", f"/admin/users/{pid}", token=adm)
check(det["verification"] and det["verification"]["status"] == "verified", "user detail shows verification", det.get("verification"))
call("POST", f"/admin/users/{pid}/suspend", {"reason": "admin flows test"}, adm)
call("GET", "/feed?mode=discover", token=pTok, expect=(401, 403))
check(True, "suspended user is locked out")
call("POST", f"/admin/users/{pid}/unsuspend", {}, adm)
call("GET", "/feed?mode=discover", token=pTok)
check(True, "unsuspended user works again")

# ---------------------------------------------------------------- player verification: location identity
notice = call("GET", "/policies/location_identity_notice")
ticket = call("POST", "/media/uploads", {"purpose": "location_proof", "contentType": "image/jpeg", "sizeBytes": len(JPEG)}, pTok)
call(ticket.get("method", "PUT"), None, raw_url=ticket["uploadUrl"], raw_body=JPEG,
     headers=ticket.get("headers") or {"Content-Type": "image/jpeg"}, expect=(200, 201, 204))
call("POST", f"/media/uploads/{ticket['uploadId']}/confirm", {}, pTok)
boot = call("GET", "/bootstrap")
proof_types = (boot.get("config") or {}).get("location.residence_proof_types") or ["rent_agreement"]
li = call("POST", "/me/location-identities", {
    "identityType": "residence", "districtId": 4, "proofType": proof_types[0], "proofUploadId": ticket["uploadId"],
    "proofHolder": "self", "noticePolicyDocumentId": notice["id"]}, pTok, expect=(200, 201))
check(li["status"] == "pending", "player submits a residence proof (pending)", li)
pend = items(call("GET", "/admin/location-identities?status=pending", token=adm))
mine = [x for x in pend if x["id"] == li["id"]]
check(mine, "admin sees it in the pending queue", pend[:3])
ld = call("GET", f"/admin/location-identities/{li['id']}", token=adm)
check(json.dumps(ld).find("http") >= 0, "admin can open the proof (signed URL)")
call("POST", f"/admin/location-identities/{li['id']}/approve", {"note": "ok"}, adm)
mineNow = call("GET", "/me/location-identities", token=pTok)
check("verified" in json.dumps(mineNow), "approved: player's residence is verified", mineNow)

# ---------------------------------------------------------------- organiser verification
organiser_submit(oTok, f"Club {RUN}")
subs = items(call("GET", "/admin/organisers?status=submitted", token=adm))
check(any(s["user"]["id"] == oid for s in subs), "organiser appears in the review queue", subs[:3])
call("POST", f"/admin/organisers/{oid}/reject", {"note": "Please add the club registration number"}, adm)
v = call("GET", "/organiser/verification", token=oTok)
check(v["status"] == "rejected", "admin rejects with a note -> organiser sees rejected", v["status"])
organiser_submit(oTok, f"Club {RUN} (reg 123)")
call("POST", f"/admin/organisers/{oid}/approve", {"note": "ok"}, adm)
check(call("GET", "/organiser/verification", token=oTok)["status"] == "approved", "resubmitted -> approved")

# ---------------------------------------------------------------- tournament management
def cfg(key, category):
    return {c["key"]: c for c in call("GET", f"/admin/config?category={category}", token=adm)}[key]


appr = cfg("event.require_admin_approval", "event")
if appr["value"] is not True:
    call("PUT", "/admin/config/event.require_admin_approval", {"value": True, "version": appr["version"]}, adm)
e1, o1 = new_event(oTok, f"Review Cup {RUN}")
check(call("GET", f"/events/{e1}", token=oTok)["status"] == "pending_review", "paid event waits for admin review")
queue = items(call("GET", "/admin/events?status=pending_review", token=adm))
check(any(e["id"] == e1 for e in queue), "admin sees it in pending review", queue[:3])
call("POST", f"/admin/events/{e1}/approve", {}, adm)
check(call("GET", f"/events/{e1}", token=pTok)["status"] == "published", "admin approves -> published")
call("POST", f"/admin/events/{e1}/feature", {}, adm)
feat = call("GET", "/events/featured", token=pTok)
check(any(e["id"] == e1 for e in items(feat)), "featured event shows in players' featured list", feat)
call("POST", f"/admin/events/{e1}/unfeature", {}, adm)
check(all(e["id"] != e1 for e in items(call("GET", "/events/featured", token=pTok))), "unfeatured -> gone from featured")
call("POST", f"/admin/events/{e1}/remove", {"reason": "admin flows test"}, adm)
check(call("GET", f"/admin/events/{e1}", token=adm)["status"] == "removed", "admin removes a published event")
call("GET", f"/events/{e1}", token=pTok, expect=(404, 410))
check(True, "removed event is hidden from players")
e2, o2 = new_event(oTok, f"Reject Cup {RUN}")
call("POST", f"/admin/events/{e2}/reject", {"reason": "Venue details missing"}, adm)
check(call("GET", f"/events/{e2}", token=oTok)["status"] == "rejected", "admin rejects a pending event")

# ---------------------------------------------------------------- payments and listing fees
orders = items(call("GET", f"/admin/listing-orders?eventId={e2}", token=adm))
o2row = [o for o in orders if o["orderId"] == o2]
check(o2row and o2row[0]["status"] in ("refunded", "partially_refunded", "paid"), "listing order listed for the admin", orders)
pct = cfg("listing.refund.on_admin_rejection_percent", "listing")["value"]
if pct and pct > 0:
    check(o2row[0]["refundPaise"] > 0, f"rejection refunds {pct}% of the listing fee", o2row[0])
else:
    check(o2row[0]["refundPaise"] == 0, "rejection refund is 0% by config", o2row[0])
prev = call("POST", "/admin/fee-rules/preview", {"sportId": 1, "eventType": "tournament", "entryFeePaise": 50000, "maxSlots": 16}, adm)
check(prev["totalPaise"] > 0 and prev["totalPaise"] == prev["baseFeePaise"] + prev["gstPaise"], "fee preview = base + GST", prev)
check(len(call("GET", "/admin/fee-rules", token=adm)) >= 1, "fee rules listed")

# ---------------------------------------------------------------- reports and moderation
post = call("POST", "/posts", {"body": f"Report me {RUN}"}, pTok)
rep = call("POST", "/reports", {"targetType": "post", "targetId": post["id"], "reason": "spam"}, rTok, expect=(200, 201))
openr = items(call("GET", "/admin/reports?status=open&targetType=post", token=adm))
mine = [x for x in openr if x["id"] == rep["id"]]
check(mine and mine[0]["targetPreview"]["ownerId"] == pid, "admin sees the report with a preview", openr[:2])
call("POST", f"/admin/reports/{rep['id']}/resolve", {"action": "hide", "note": "spam"}, adm)
call("GET", f"/posts/{post['id']}", token=rTok, expect=404)
check(True, "resolve 'hide' -> post hidden from users")
acts = items(call("GET", f"/admin/moderation/actions?targetType=post&targetId={post['id']}", token=adm))
check(any(a["action"] == "hide" for a in acts), "moderation action recorded", acts)
rep2 = call("POST", "/reports", {"targetType": "user", "targetId": oid, "reason": "other", "details": "test"}, rTok, expect=(200, 201))
call("POST", f"/admin/reports/{rep2['id']}/resolve", {"action": "dismiss", "note": "not a violation"}, adm)
done = [x for x in items(call("GET", "/admin/reports?status=dismissed", token=adm)) if x["id"] == rep2["id"]]
check(done, "resolve 'dismiss' -> report dismissed")

# ---------------------------------------------------------------- application configuration
entry = cfg("feed.post_max_length", "feed") if any(c["key"] == "feed.post_max_length" for c in call("GET", "/admin/config?category=feed", token=adm)) else None
if entry is None:
    entry = call("GET", "/admin/config", token=adm)
    entry = [c for c in entry if c.get("valueType") == "int"][0]
key, old = entry["key"], entry["value"]
upd = call("PUT", f"/admin/config/{key}", {"value": old + 1, "version": entry["version"]}, adm)
check(upd["value"] == old + 1, f"config {key} updated", upd)
call("PUT", f"/admin/config/{key}", {"value": "not-a-number", "version": upd["version"]}, adm, expect=422)
check(True, "config value of the wrong type -> 422")
call("PUT", f"/admin/config/{key}", {"value": old, "version": upd["version"]}, adm)
hist = call("GET", f"/admin/config/{key}/history", token=adm)
check(len(hist) >= 2, "config history records the change and the restore", hist[:2])

# ---------------------------------------------------------------- role-based permissions
role = call("POST", "/admin/roles", {"name": f"viewer{RUN}", "description": "test", "permissions": ["users.read"]}, adm, expect=(200, 201))
made = call("POST", "/admin/admins", {"email": f"viewer{RUN}@athlix.local", "name": "Viewer", "roleId": role["id"]}, adm, expect=(200, 201))
vlogin = admin_login(f"viewer{RUN}@athlix.local", made["temporaryPassword"], made["totpUri"])
vtok = vlogin["tokens"]["accessToken"]
call("GET", "/admin/users", token=vtok)
call("GET", "/admin/config", token=vtok, expect=403)
call("POST", f"/admin/users/{pid}/suspend", {"reason": "x"}, vtok, expect=403)
call("GET", "/admin/reports", token=vtok, expect=403)
check(True, "limited admin: users.read allowed; config, suspend, reports -> 403")
call("PUT", f"/admin/admins/{made['admin']['id']}", {"status": "disabled"}, adm)
call("GET", "/admin/me", token=vtok, expect=(401, 403))
check(True, "disabled admin's session stops working")
call("GET", "/admin/users", expect=401)
check(True, "admin API without a token -> 401")
call("GET", "/admin/users", token=pTok, expect=401)
check(True, "a user token cannot call the admin API (401)")

# ---------------------------------------------------------------- audit log
log = items(call("GET", "/admin/audit-log", token=adm))
acts = {a["action"] for a in log[:200]}
check(any("suspend" in a for a in acts) and any("approve" in a for a in acts), "audit log records suspend and approvals",
      sorted(acts)[:20])

print(f"\nALL {ok_count} CHECKS PASSED")
