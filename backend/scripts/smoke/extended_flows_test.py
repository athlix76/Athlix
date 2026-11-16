"""Extended end-to-end checks against a LOCAL Athlix API (Phase 4). Prints no secrets.

Covers what smoke_test.py, core_flows_test.py and admin_flows_test.py do not:
authentication edge cases (wrong OTP, refresh rotation and reuse detection, logout, bad tokens),
minors (guardian OTP consent, DM policy, default profile visibility, no ads), profile stats and
achievements, privacy settings, block, photo posts, communities, organiser tools (waitlist,
mark paid, event group chat, promotion on withdraw), coach consultations, premium (mock) and
ads (switched on for the test, then restored), and API error handling.

usage (same admin env vars as smoke_test.py, a super_admin):
  SMOKE_ADMIN_EMAIL=... SMOKE_ADMIN_PASSWORD=... SMOKE_ADMIN_TOTP_URI='otpauth://...' \
    python3 -I extended_flows_test.py http://127.0.0.1:8080/v1
"""
import base64, hashlib, hmac, json, os, struct, sys, time, urllib.error, urllib.parse, urllib.request, uuid
from datetime import datetime, timedelta, timezone

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8080/v1"
if not BASE.startswith(("http://127.0.0.1", "http://localhost")):
    sys.exit("extended flows test runs against a local API only")
RUN = "%06d" % (int(time.time()) % 1000000)
ok_count = 0
JPEG = base64.b64decode(
    "/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAP//////////////////////////////////////////////////////////////"
    "////////////////////////wgALCAABAAEBAREA/8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABPxA=")
restore = []  # (callable) run at the end, also on failure


def raw_call(method, path, body=None, token=None, raw_url=None, raw_body=None, headers=None):
    url = raw_url or BASE + path
    data = raw_body if raw_body is not None else (None if body is None else json.dumps(body).encode())
    req = urllib.request.Request(url, data=data, method=method)
    if raw_body is None or headers is None:
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
    return status, out


def call(method, path, body=None, token=None, expect=200, **kw):
    status, out = raw_call(method, path, body, token, **kw)
    expected = expect if isinstance(expect, tuple) else (expect,)
    if status not in expected:
        fail(f"{method} {path or kw.get('raw_url')}: want {expect} got {status}: {json.dumps(out)[:700]}")
    return out.get("data", out) if isinstance(out, dict) else out


def fail(msg):
    print("FAIL", msg)
    for fn in reversed(restore):
        try:
            fn()
        except SystemExit:
            pass
    sys.exit(1)


def items(page):
    return page["items"] if isinstance(page, dict) and "items" in page else page


def check(cond, label, detail=""):
    global ok_count
    if not cond:
        fail(label + " " + (json.dumps(detail, default=str)[:700] if detail != "" else ""))
    ok_count += 1
    print("ok  ", label)


def eventually(fn, label, tries=20):
    last = None
    for _ in range(tries):
        good, last = fn()
        if good:
            return check(True, label)
        time.sleep(0.25)
    check(False, label, last)


def totp(uri):
    secret = urllib.parse.parse_qs(urllib.parse.urlparse(uri).query)["secret"][0]
    key = base64.b32decode(secret + "=" * (-len(secret) % 8))
    h = hmac.new(key, struct.pack(">Q", int(time.time()) // 30), hashlib.sha1).digest()
    o = h[-1] & 15
    return "%06d" % ((struct.unpack(">I", h[o:o + 4])[0] & 0x7FFFFFFF) % 1000000)


def otp_login(mobile):
    r = call("POST", "/auth/otp/request", {"mobile": mobile})
    return call("POST", "/auth/otp/verify", {"requestId": r["requestId"], "otp": "123456"})


def onboard(mobile, role, name, outcome="adult", district=26):
    v = otp_login(mobile)
    tok = v["tokens"]["accessToken"]
    call("POST", "/onboarding/role", {"role": role}, tok)
    me = call("GET", "/me", token=tok)
    if me["pendingPolicies"]:
        call("POST", "/me/policies/accept", {"policyDocumentIds": [p["id"] for p in me["pendingPolicies"]]}, tok)
    call("POST", "/verification/mock/complete", {"outcome": outcome, "nativeDistrictId": district}, tok)
    if outcome != "adult":
        return tok, call("GET", "/me", token=tok), v
    me = call("POST", "/onboarding/profile", {"displayName": name, "sportId": 1, "city": "Chennai", "state": "Tamil Nadu"}, tok)
    return tok, me, v


def upload(tok, purpose):
    t = call("POST", "/media/uploads", {"purpose": purpose, "contentType": "image/jpeg", "sizeBytes": len(JPEG)}, tok)
    call(t.get("method", "PUT"), None, raw_url=t["uploadUrl"], raw_body=JPEG,
         headers=t.get("headers") or {"Content-Type": "image/jpeg"}, expect=(200, 201, 204))
    call("POST", f"/media/uploads/{t['uploadId']}/confirm", {}, tok)
    return t["uploadId"]


def admin_cfg(adm, key):
    cat = key.split(".")[0]
    rows = call("GET", f"/admin/config?category={cat}", token=adm)
    return {c["key"]: c for c in rows}[key]


def set_cfg(adm, key, value):
    e = admin_cfg(adm, key)
    if e["value"] != value:
        call("PUT", f"/admin/config/{key}", {"value": value, "version": e["version"]}, adm)
    return e["value"]


def dm(tok, other):
    return raw_call("POST", "/chat/direct", {"userId": other}, tok)[0]


# ============================================================ setup
call("GET", "/health")
time.sleep(31 - time.time() % 30)
adm = call("POST", "/admin/auth/login", {"email": os.environ["SMOKE_ADMIN_EMAIL"], "password": os.environ["SMOKE_ADMIN_PASSWORD"],
                                         "totpCode": totp(os.environ["SMOKE_ADMIN_TOTP_URI"])})["tokens"]["accessToken"]
p1Tok, p1, p1Login = onboard("7" + RUN + "401", "player", f"ExtP1{RUN}")
p2Tok, p2, _ = onboard("7" + RUN + "402", "player", f"ExtP2{RUN}")
cTok, coach, _ = onboard("7" + RUN + "403", "coach", f"ExtCoach{RUN}")
oTok, org, _ = onboard("7" + RUN + "404", "organiser", f"ExtOrg{RUN}")
P1, P2, C, O = p1["id"], p2["id"], coach["id"], org["id"]

# ============================================================ authentication
r = call("POST", "/auth/otp/request", {"mobile": "7" + RUN + "409"})
st, _ = raw_call("POST", "/auth/otp/verify", {"requestId": r["requestId"], "otp": "000000"})
check(st in (400, 401, 422), "wrong OTP is rejected", st)
st, _ = raw_call("POST", "/auth/otp/request", {"mobile": "7" + RUN + "409"})
check(st == 429, "OTP resend inside the cooldown -> 429", st)
old_refresh = p1Login["tokens"]["refreshToken"]
pair = call("POST", "/auth/refresh", {"refreshToken": old_refresh})
check(pair["refreshToken"] != old_refresh and pair["accessToken"], "refresh rotates the token pair")
call("POST", "/auth/refresh", {"refreshToken": old_refresh}, expect=401)
call("POST", "/auth/refresh", {"refreshToken": pair["refreshToken"]}, expect=401)
check(True, "reusing an old refresh token -> 401 and the whole session family is revoked")
time.sleep(31)
p1Login = otp_login("7" + RUN + "401")
p1Tok = p1Login["tokens"]["accessToken"]
call("POST", "/auth/logout", {"refreshToken": p1Login["tokens"]["refreshToken"]}, p1Tok, expect=(200, 204))
call("POST", "/auth/refresh", {"refreshToken": p1Login["tokens"]["refreshToken"]}, expect=401)
check(True, "logout revokes the refresh token")
call("GET", "/me", token="not-a-jwt", expect=401)
call("GET", "/me", token=p1Tok[:-4] + "AAAA", expect=401)
check(True, "garbage or tampered access token -> 401")
time.sleep(31)
p1Tok = otp_login("7" + RUN + "401")["tokens"]["accessToken"]

# ============================================================ minors and guardian consent
mTok, m, _ = onboard("7" + RUN + "405", "player", f"ExtMinor{RUN}", outcome="minor")
check(m["isMinor"] and m["onboardingStep"] == "guardian", "minor verification -> guardian consent step", m)
gpol = call("GET", "/policies/guardian_consent")
g = call("POST", "/onboarding/guardian/request", {"guardianName": "Parent", "guardianMobile": "7" + RUN + "499",
                                                  "relation": "mother", "policyDocumentId": gpol["id"]}, mTok)
check(g["guardianMobileMasked"].startswith("*"), "guardian OTP sent (mobile masked)", g)
m = call("POST", "/onboarding/guardian/verify", {"consentId": g["consentId"], "otp": "123456"}, mTok)
check(m["onboardingStep"] != "guardian", "guardian OTP verified -> consent granted", m.get("onboardingStep"))
m = call("POST", "/onboarding/profile", {"displayName": f"ExtMinor{RUN}", "sportId": 1, "city": "Chennai", "state": "Tamil Nadu"}, mTok)
M = m["id"]
check(m["onboardingStep"] == "done", "minor finishes onboarding")
yTok, y, _ = onboard("7" + RUN + "406", "player", "x", outcome="too_young")
check(y["verificationStatus"] != "verified" and y["onboardingStep"] != "done", "below minimum age -> not allowed in", y)
check(dm(p1Tok, M) in (403, 404), "adult stranger cannot DM a minor (minor.dm_policy = mutual_follow)")
mp = call("GET", f"/profiles/{M}", token=p1Tok)
check(mp["restricted"], "minor profile is followers-only to strangers", mp)
call("POST", f"/users/{M}/follow", token=p1Tok)
call("POST", f"/users/{P1}/follow", token=mTok)
check(dm(p1Tok, M) in (200, 201), "after a mutual follow the DM is allowed")

# ============================================================ profile: stats, achievements, visibility
sport = [s for s in call("GET", "/sports") if s["id"] == 1][0]
tpl = sport["statTemplates"][0]
field = [f for f in tpl["fields"] if f["type"] in ("int", "decimal")][0]
st = call("POST", "/me/stats", {"sportId": 1, "templateId": tpl["id"], "season": "2025-26", "stats": {field["key"]: 812}}, p1Tok, expect=(200, 201))
check(st["stats"][field["key"]] == 812, "stat entry saved from the sport template", st)
call("POST", "/me/stats", {"sportId": 1, "templateId": tpl["id"], "season": "2025-26", "stats": {"not_a_field": 1}}, p1Tok, expect=422)
check(True, "unknown stat key -> 422")
ach = call("POST", "/me/achievements", {"sportId": 1, "title": "State U-19 champion", "level": "state", "achievedOn": "2025-01-20"}, p1Tok, expect=(200, 201))
achs = call("GET", f"/profiles/{P1}/achievements", token=p2Tok)
check(any(a["id"] == ach["id"] for a in achs), "achievement visible on the profile", achs)
call("PUT", "/me/profile", {"visibility": "private"}, p2Tok)
check(call("GET", f"/profiles/{P2}", token=p1Tok)["restricted"], "private profile is restricted to others")
call("PUT", "/me/profile", {"visibility": "public"}, p2Tok)

# ============================================================ privacy settings
call("PUT", "/me/settings", {"privacy": {"allowDmsFrom": "nobody", "showInSearch": False}}, cTok)
found = items(call("GET", "/search/people?" + urllib.parse.urlencode({"q": f"ExtCoach{RUN}"}), token=p1Tok))
check(all(u["id"] != C for u in found), "showInSearch=false hides the coach from search", found)
check(dm(p1Tok, C) in (403, 404), "allowDmsFrom=nobody blocks new DMs")
call("PUT", "/me/settings", {"privacy": {"allowDmsFrom": "everyone", "showInSearch": True}}, cTok)
check(dm(p1Tok, C) in (200, 201), "settings restored: DM allowed again")

# ============================================================ block
call("POST", f"/users/{P1}/block", token=p2Tok, expect=(200, 204))
call("GET", f"/profiles/{P2}", token=p1Tok, expect=404)
check(dm(p1Tok, P2) in (403, 404), "blocked: profile hidden (404) and no DM")
check(any(u["id"] == P1 for u in items(call("GET", "/me/blocks", token=p2Tok))), "block listed in /me/blocks")
call("DELETE", f"/users/{P1}/block", token=p2Tok, expect=(200, 204))
call("GET", f"/profiles/{P2}", token=p1Tok)
check(True, "unblock restores the profile")

# ============================================================ photo post
img = upload(p1Tok, "post_image")
post = call("POST", "/posts", {"body": "With a photo", "mediaUploadIds": [img]}, p1Tok)
check("http" in json.dumps(post), "post with a photo has a signed image URL", post)

# ============================================================ communities
com = call("POST", "/communities", {"title": f"Ext Open {RUN}", "description": "test", "sportId": 1, "city": "Chennai",
                                    "state": "Tamil Nadu", "visibility": "public", "joinPolicy": "open"}, p1Tok, expect=(200, 201))
j = call("POST", f"/communities/{com['id']}/join", {}, p2Tok)
check(j["myStatus"] == "active", "open community: join is immediate", j)
msg = call("POST", f"/chat/conversations/{com['id']}/messages", {"clientMsgId": str(uuid.uuid4()), "kind": "text", "body": "hello group"}, p2Tok, expect=(200, 201))
check(msg["id"] > 0, "member posts in the community chat")
appr = call("POST", "/communities", {"title": f"Ext Approval {RUN}", "description": "test", "sportId": 1, "city": "Chennai",
                                     "state": "Tamil Nadu", "visibility": "public", "joinPolicy": "approval"}, p1Tok, expect=(200, 201))
check(call("POST", f"/communities/{appr['id']}/join", {}, cTok)["myStatus"] == "pending", "approval community: join is pending")
reqs = items(call("GET", f"/communities/{appr['id']}/join-requests", token=p1Tok))
check(any(r["user"]["id"] == C for r in reqs), "owner sees the join request", reqs)
call("POST", f"/communities/{appr['id']}/join-requests/{C}/approve", token=p1Tok, expect=(200, 204))
check(call("GET", f"/communities/{appr['id']}", token=cTok)["myStatus"] == "active", "owner approves -> member")
call("POST", f"/communities/{com['id']}/members/{P2}/ban", token=p1Tok, expect=(200, 204))
st, _ = raw_call("POST", f"/chat/conversations/{com['id']}/messages", {"clientMsgId": str(uuid.uuid4()), "kind": "text", "body": "x"}, p2Tok)
check(st in (403, 404), "banned member can no longer post", st)
call("POST", f"/admin/communities/{appr['id']}/remove", {"reason": "test"}, adm, expect=(200, 204))
call("GET", f"/communities/{appr['id']}", token=cTok, expect=404)
check(True, "admin removes a community -> gone for members")

# ============================================================ organiser tools: waitlist, mark paid, group chat
now = datetime.now(timezone.utc)
iso = lambda d: d.strftime("%Y-%m-%dT%H:%M:%SZ")
call("GET", "/organiser/verification", token=oTok)
vsteps = call("GET", "/organiser/verification", token=oTok)["steps"]
steps = [{"stepId": s["stepId"], "checked": True} if s["inputType"] == "checkbox" else {"stepId": s["stepId"], "valueText": "Ext Club"}
         for s in vsteps if s["inputType"] in ("checkbox", "text")]
call("PUT", "/organiser/verification", {"orgName": "Ext Club", "orgType": "club", "steps": steps}, oTok)
call("POST", "/organiser/verification/submit", {}, oTok)
call("POST", f"/admin/organisers/{O}/approve", {"note": "ok"}, adm)
ev = call("POST", "/events", {
    "sportId": 1, "title": f"One Slot Cup {RUN}", "description": "waitlist test", "eventType": "tournament",
    "participantType": "individual", "competitionLevel": "open", "genderCategory": "open", "venueName": "Ground",
    "addressLine": "Main Road", "city": "Chennai", "state": "Tamil Nadu", "pincode": "600001",
    "startsAt": iso(now + timedelta(days=20)), "endsAt": iso(now + timedelta(days=21)),
    "registrationDeadline": iso(now + timedelta(days=10)), "entryFeePaise": 10000, "paymentModes": ["cash"],
    "paymentInstructions": "Cash at the ground", "maxSlots": 1, "contactName": "Ravi", "contactPhone": "9876543210",
    "category": "private"}, oTok, expect=201)
E = ev["id"]
order = call("POST", f"/events/{E}/listing/order", {}, oTok)
call("POST", "/listing/mock/pay", {"orderId": order["orderId"]}, oTok)
if call("GET", f"/events/{E}", token=oTok)["status"] == "pending_review":
    call("POST", f"/admin/events/{E}/approve", {}, adm)
r1 = call("POST", f"/events/{E}/registrations", {}, p1Tok, expect=201)
r2 = call("POST", f"/events/{E}/registrations", {}, p2Tok, expect=201)
check(r1["status"] == "registered" and r2["status"] == "waitlisted", "1 slot: first registers, second is waitlisted", [r1["status"], r2["status"]])
eventually(lambda: (raw_call("GET", f"/events/{E}/group", token=p1Tok)[0] == 200, None), "registered player is in the event group chat")
check(raw_call("GET", f"/events/{E}/group", token=p2Tok)[0] == 404, "waitlisted player is not in the group")
regs = items(call("GET", f"/events/{E}/registrations", token=oTok))
check(len(regs) == 2, "organiser sees both registrations", regs)
upd = call("PATCH", f"/events/{E}/registrations/{r1['id']}", {"paymentStatus": "paid_offline", "paymentNote": "cash"}, oTok)
check(upd["paymentStatus"] == "paid_offline", "organiser marks a registration paid (offline)")
mine = [x for x in items(call("GET", "/me/registrations", token=p1Tok)) if x["eventId"] == E]
check(mine and mine[0]["paymentStatus"] == "paid_offline", "player sees the payment as marked", mine)
call("DELETE", f"/events/{E}/registrations/me", token=p1Tok, expect=(200, 204))
eventually(lambda: ((lambda xs: bool(xs) and xs[0]["status"] == "registered")(
    [x for x in items(call("GET", "/me/registrations", token=p2Tok)) if x["eventId"] == E]), None),
    "first player withdraws -> waitlisted player is promoted")
eventually(lambda: (raw_call("GET", f"/events/{E}/group", token=p2Tok)[0] == 200, None), "promoted player joins the group chat")

# ============================================================ coach consultations
call("PUT", "/me/consultation", {"offered": True, "description": "Batting technique", "feeNote": "Rs 500/hour, paid to me",
                                 "modes": ["online"]}, cTok)
offer = call("GET", f"/coaches/{C}/consultation", token=p1Tok)
check(offer["offered"] and offer["canRequest"], "player sees the coach's consultation offer", offer)
cr = call("POST", f"/coaches/{C}/consultation-requests", {"message": "Can you help my cover drive?", "preferredMode": "online"}, p1Tok, expect=(200, 201))
pending = items(call("GET", "/me/consultation-requests?as=coach&status=pending", token=cTok))
check(any(x["id"] == cr["id"] for x in pending), "coach sees the pending request", pending)
acc = call("POST", f"/consultation-requests/{cr['id']}/respond", {"accept": True, "note": "Sure"}, cTok)
check(acc["status"] == "accepted", "coach accepts", acc)
done = call("POST", f"/consultation-requests/{cr['id']}/complete", {}, cTok)
check(done["status"] == "completed", "coach marks it completed", done)

# ============================================================ premium (mock purchase)
was = set_cfg(adm, "premium.enabled", False)
check(call("GET", "/premium/plans", token=p1Tok) == [], "premium.enabled=false -> no plans offered")
set_cfg(adm, "premium.enabled", True)
restore.append(lambda: set_cfg(adm, "premium.enabled", was))
time.sleep(16)  # settings cache is 15 s
plans = call("GET", "/premium/plans?audience=player", token=p1Tok)
check(len(plans) >= 1, "premium on -> player plans listed", plans)
sub = call("POST", "/premium/mock/activate", {"planId": plans[0]["id"]}, p1Tok, expect=(200, 201))
check(sub["status"] in ("active", "grace"), "mock purchase activates the plan", sub)
set_cfg(adm, "premium.enabled", was)
restore.pop()

# ============================================================ ads
was_ads = set_cfg(adm, "ads.enabled", False)
restore.append(lambda: set_cfg(adm, "ads.enabled", was_ads))
ad = call("POST", "/admin/ads", {"advertiserName": "Ext Sports", "title": f"Ext ad {RUN}", "body": "Gear sale",
                                 "targetUrl": "https://example.com/sale", "placement": "home_feed", "priority": 100,
                                 "startsAt": iso(now - timedelta(hours=1)), "endsAt": iso(now + timedelta(days=1)),
                                 "status": "active"}, adm, expect=(200, 201))
time.sleep(16)
check(call("GET", "/ads?placement=home_feed", token=p1Tok) == [], "ads.enabled=false -> no ads")
set_cfg(adm, "ads.enabled", True)
time.sleep(16)
ads = call("GET", "/ads?placement=home_feed", token=p1Tok)
check(any(a["id"] == ad["id"] for a in ads), "ads on -> active campaign is served", ads)
call("POST", f"/ads/{ad['id']}/impression", token=p1Tok, expect=(200, 204))
call("POST", f"/ads/{ad['id']}/click", token=p1Tok, expect=(200, 204))
check(call("GET", "/ads?placement=home_feed", token=mTok) == [], "minors get no ads (ads.show_to_minors=false)")
stats = call("GET", f"/admin/ads/{ad['id']}/stats", token=adm)
check(sum(s.get("impressions", 0) for s in stats) >= 1 and sum(s.get("clicks", 0) for s in stats) >= 1, "impression and click counted", stats)
call("PUT", f"/admin/ads/{ad['id']}", {"status": "ended"}, adm)
set_cfg(adm, "ads.enabled", was_ads)
restore.pop()

# ============================================================ error handling
st, out = raw_call("POST", "/posts", {}, p1Tok)
check(st == 422 and "error" in out, "empty post -> 422 with an error envelope", [st, out])
st, out = raw_call("POST", "/events", {"title": ""}, oTok)
check(st == 422 and out["error"].get("fields"), "invalid event -> 422 with field errors", [st, out])
st, out = raw_call("POST", "/posts", None, p1Tok, raw_body=b"{not json", headers={"Content-Type": "application/json"})
check(st == 400, "malformed JSON -> 400", [st, out])
st, out = raw_call("GET", "/no/such/route", token=p1Tok)
check(st == 404 and out["error"]["code"] == "NOT_FOUND", "unknown route -> 404 JSON envelope", [st, out])
st, out = raw_call("POST", "/posts", None, p1Tok, raw_body=b'{"body":"' + b"a" * (1 << 20) + b'"}', headers={"Content-Type": "application/json"})
check(st == 413 or st == 400, "body over 1 MB is refused", st)
st, out = raw_call("GET", "/events/999999999", token=p1Tok)
check(st == 404 and "999999999" not in json.dumps(out), "missing event -> 404 without echoing internals", [st, out])

print(f"\nALL {ok_count} CHECKS PASSED")
