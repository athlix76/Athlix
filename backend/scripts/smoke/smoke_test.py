"""End-to-end smoke test against a LOCAL Athlix API (never dev/prod). Prints no secrets.

usage (see README.md):
  SMOKE_ADMIN_EMAIL=... SMOKE_ADMIN_PASSWORD=... SMOKE_ADMIN_TOTP_URI='otpauth://...' \
    python3 -I smoke_test.py http://127.0.0.1:8080/v1
"""
import base64, hashlib, hmac, json, os, struct, sys, time, urllib.error, urllib.parse, urllib.request
from datetime import datetime, timedelta, timezone

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8080/v1"
if not BASE.startswith(("http://127.0.0.1", "http://localhost")):
    sys.exit("smoke test runs against a local API only")
ADMIN_EMAIL = os.environ["SMOKE_ADMIN_EMAIL"]
ok_count = 0
RUN = "%06d" % (int(time.time()) % 1000000)  # unique mobiles per run (OTP resend cooldown)


def call(method, path, body=None, token=None, expect=200):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("content-type", "application/json")
    if token:
        req.add_header("authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req) as r:
            status, raw = r.status, r.read()
    except urllib.error.HTTPError as e:
        status, raw = e.code, e.read()
    out = json.loads(raw) if raw else {}
    if status != expect:
        print(f"FAIL {method} {path}: want {expect} got {status}: {json.dumps(out)[:600]}")
        sys.exit(1)
    return out.get("data", out)


def items(page):
    return page["items"] if isinstance(page, dict) else page


def check(cond, label, detail=""):
    global ok_count
    if not cond:
        print("FAIL", label, detail)
        sys.exit(1)
    ok_count += 1
    print("ok  ", label)


def login(mobile):
    r = call("POST", "/auth/otp/request", {"mobile": mobile})
    v = call("POST", "/auth/otp/verify", {"requestId": r["requestId"], "otp": "123456"})
    return v["tokens"]["accessToken"], v["user"]


def onboard(mobile, role, district):
    tok, me = login(mobile)
    call("POST", "/onboarding/role", {"role": role}, tok)
    me = call("GET", "/me", token=tok)
    if me["pendingPolicies"]:
        call("POST", "/me/policies/accept", {"policyDocumentIds": [p["id"] for p in me["pendingPolicies"]]}, tok)
    call("POST", "/verification/mock/complete", {"outcome": "adult", "nativeDistrictId": district}, tok)
    me = call("POST", "/onboarding/profile", {"displayName": "Smoke " + mobile[-2:], "sportId": 1, "city": "Chennai",
                                              "state": "Tamil Nadu"}, tok)
    return tok, me


def totp(uri):
    secret = urllib.parse.parse_qs(urllib.parse.urlparse(uri).query)["secret"][0]
    key = base64.b32decode(secret + "=" * (-len(secret) % 8))
    h = hmac.new(key, struct.pack(">Q", int(time.time()) // 30), hashlib.sha1).digest()
    o = h[-1] & 15
    return "%06d" % ((struct.unpack(">I", h[o:o + 4])[0] & 0x7FFFFFFF) % 1000000)


# --- users ---
call("GET", "/health")
orgTok, org = onboard("9"+RUN+"101", "organiser", 26)
check(org["onboardingStep"] == "done" and org["verificationStatus"] == "verified", "organiser onboarded + verified", org)
check(all(p["docType"] not in ("guardian_consent", "location_identity_notice") for p in org["pendingPolicies"]),
      "no guardian/location notice in pendingPolicies")
thanjTok, _ = onboard("9"+RUN+"102", "player", 26)      # native Thanjavur
coimbTok, _ = onboard("9"+RUN+"103", "player", 4)       # native Coimbatore
ids = call("GET", "/me/location-identities", token=thanjTok)
check("native" in json.dumps(ids) and "Thanjavur" in json.dumps(ids), "DigiLocker district became native identity", ids)

# --- admin ---
pw = os.environ["SMOKE_ADMIN_PASSWORD"]
uri = os.environ["SMOKE_ADMIN_TOTP_URI"]
time.sleep(31 - time.time() % 30)  # fresh TOTP window (codes are single-use per window)
adm = call("POST", "/admin/auth/login", {"email": ADMIN_EMAIL, "password": pw, "totpCode": totp(uri)})["tokens"]["accessToken"]
call("POST", "/admin/auth/login", {"email": ADMIN_EMAIL, "password": "wrong-password", "totpCode": "000000"}, expect=401)
check(True, "admin login with password + TOTP; bad password rejected")

# --- organiser approval ---
v = call("GET", "/organiser/verification", token=orgTok)
steps = []
for s in v["steps"]:
    if s["inputType"] == "checkbox":
        steps.append({"stepId": s["stepId"], "checked": True})
    elif s["inputType"] == "text":
        steps.append({"stepId": s["stepId"], "valueText": "Smoke Sports Club"})
docSteps = [s for s in v["steps"] if s["inputType"] == "document" and s["isRequired"]]
if docSteps:
    print("note: required document steps exist; skipping organiser upload (needs S3) and approving via admin")
call("PUT", "/organiser/verification", {"orgName": "Smoke Club", "orgType": "club", "steps": steps}, orgTok)
if not docSteps:
    call("POST", "/organiser/verification/submit", {}, orgTok)
call("POST", f"/admin/organisers/{org['id']}/approve", {"note": "smoke"}, adm,
     expect=200 if not docSteps else 409)
if docSteps:
    sys.exit("organiser needs a document upload; extend the smoke test")
check(call("GET", "/organiser/verification", token=orgTok)["status"] == "approved", "organiser approved by admin")

# --- listing fee from config ---
q = call("GET", "/listing/quote?sportId=1&eventType=tournament&entryFeePaise=50000&maxSlots=16", token=orgTok)
check(q["baseFeePaise"] == 40000 and q["gstPaise"] == 7200 and q["totalPaise"] == 47200, "fee = 5% x 500 x 16 + 18% GST = 472.00", q)
rule = q["feeRuleId"]
call("PUT", f"/admin/fee-rules/{rule}", {"percentBp": 1000}, adm)
q2 = call("GET", "/listing/quote?sportId=1&eventType=tournament&entryFeePaise=50000&maxSlots=16", token=orgTok)
check(q2["totalPaise"] == 94400, "admin changes fee rule to 10% -> 944.00 with no restart", q2)
call("PUT", f"/admin/fee-rules/{rule}", {"percentBp": 500}, adm)
q3 = call("GET", "/listing/quote?sportId=1&eventType=tournament&entryFeePaise=0&maxSlots=16", token=orgTok)
check(q3["isFreeEvent"] and q3["baseFeePaise"] == 9900, "free event uses freeEventFeePaise", q3)

# --- config switch: admin approval off ---
def cfg_entry(key):
    return {c["key"]: c for c in call("GET", "/admin/config?category=event", token=adm)}[key]
appr = cfg_entry("event.require_admin_approval")
if appr["value"] is False:   # left over from an aborted run
    call("PUT", "/admin/config/event.require_admin_approval", {"value": True, "version": appr["version"]}, adm)
    appr = cfg_entry("event.require_admin_approval")
call("PUT", "/admin/config/event.require_admin_approval", {"value": False, "version": appr["version"]}, adm)
call("PUT", "/admin/config/event.require_admin_approval", {"value": True, "version": appr["version"]}, adm, expect=409)
check(True, "config write + stale version rejected (409)")

# --- event with location eligibility ---
now = datetime.now(timezone.utc)
iso = lambda d: d.strftime("%Y-%m-%dT%H:%M:%SZ")
ev = call("POST", "/events", {
    "sportId": 1, "title": "Thanjavur District Cup", "description": "Smoke test event",
    "eventType": "tournament", "participantType": "individual", "competitionLevel": "district",
    "genderCategory": "open", "venueName": "Ground", "addressLine": "Main Road", "city": "Thanjavur",
    "state": "Tamil Nadu", "pincode": "613001",
    "startsAt": iso(now + timedelta(days=20)), "endsAt": iso(now + timedelta(days=21)),
    "registrationDeadline": iso(now + timedelta(days=10)),
    "entryFeePaise": 50000, "paymentModes": ["cash", "upi"], "paymentInstructions": "Pay at the ground",
    "maxSlots": 16, "contactName": "Ravi", "contactPhone": "9876543210", "category": "government",
    "eligibility": {"type": "native", "scope": "district", "districtId": 26, "stateId": None, "requireVerified": True},
}, orgTok, expect=201)
eid = ev["id"]
check(ev["status"] == "draft", "event created as draft")
order = call("POST", f"/events/{eid}/listing/order", {}, orgTok)
check(order["quote"]["totalPaise"] == 47200, "listing order snapshots quote 472.00", order["quote"])
paid = call("POST", "/listing/mock/pay", {"orderId": order["orderId"]}, orgTok)
check(paid["status"] == "paid", "mock payment marks order paid")
ev = call("GET", f"/events/{eid}", token=orgTok)
check(ev["status"] == "published", "approval switch off -> published directly", ev["status"])
call("PUT", "/admin/config/event.require_admin_approval", {"value": True, "version": cfg_entry("event.require_admin_approval")["version"]}, adm)

# --- eligibility ---
feedC = call("GET", "/events", token=coimbTok)
check(all(e["id"] != eid for e in items(feedC)), "Coimbatore native does not see Thanjavur-only event in feed")
allC = call("GET", "/events?showAll=true", token=coimbTok)
mine = [e for e in items(allC) if e["id"] == eid]
check(mine and not mine[0]["myEligibility"]["eligible"], "showAll=true lists it greyed (ineligible)", mine)
err = call("POST", f"/events/{eid}/registrations", {}, coimbTok, expect=403)
check(True, "Coimbatore native register -> 403 NOT_ELIGIBLE")
call("GET", f"/events/{eid}/contact", token=coimbTok, expect=403)
check(True, "ineligible user cannot read organiser contact / payment instructions")
feedT = call("GET", "/events", token=thanjTok)
check(any(e["id"] == eid for e in items(feedT)), "Thanjavur native sees it in feed")
reg = call("POST", f"/events/{eid}/registrations", {}, thanjTok, expect=201)
check(reg.get("qualifiedVia") == "native", "Thanjavur native registers, qualifiedVia native", reg)
contact = call("GET", f"/events/{eid}/contact", token=thanjTok)
check(contact.get("contactPhone") == "+919876543210", "eligible user gets organiser contact")
regs = call("GET", f"/events/{eid}/registrations", token=orgTok)
check(any("Thanjavur" in (r.get("qualifiedLabel") or "") for r in items(regs)), "organiser sees qualifiedLabel", items(regs))

# --- ownership ---
call("PUT", f"/events/{eid}", {"title": "hijack"}, coimbTok, expect=404)
call("GET", f"/events/{eid}/registrations", token=coimbTok, expect=404)
check(True, "non-owner edit / registrations list -> 404")
call("GET", "/me", expect=401)
check(True, "no token -> 401")
print(f"\nALL {ok_count} CHECKS PASSED")
