"""End-to-end check of the social core flows against a LOCAL Athlix API. Prints no secrets.

Covers what smoke_test.py does not: player and coach onboarding, profile with sports,
people search, follow, posts / feed / likes / comments, direct messages, notifications,
and ownership on posts. Needs the same local setup as smoke_test.py (ENV=local,
DEV_FIXED_OTP=123456, verification.provider = mock) but no admin account.

usage:  python3 -I core_flows_test.py http://127.0.0.1:8080/v1
"""
import json, sys, time, urllib.error, urllib.parse, urllib.request, uuid

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8080/v1"
if not BASE.startswith(("http://127.0.0.1", "http://localhost")):
    sys.exit("core flows test runs against a local API only")
RUN = "%06d" % (int(time.time()) % 1000000)  # unique mobiles and names per run
ok_count = 0


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
    expected = expect if isinstance(expect, tuple) else (expect,)
    if status not in expected:
        print(f"FAIL {method} {path}: want {expect} got {status}: {json.dumps(out)[:600]}")
        sys.exit(1)
    return out.get("data", out) if isinstance(out, dict) else out


def items(page):
    return page["items"] if isinstance(page, dict) else page


def check(cond, label, detail=""):
    global ok_count
    if not cond:
        print("FAIL", label, json.dumps(detail)[:600] if detail != "" else "")
        sys.exit(1)
    ok_count += 1
    print("ok  ", label)


def eventually(fn, label, tries=20):
    """Domain events run after commit; give in-process handlers a moment."""
    last = None
    for _ in range(tries):
        ok, last = fn()
        if ok:
            check(True, label)
            return last
        time.sleep(0.25)
    check(False, label, last)


def onboard(mobile, role, name):
    r = call("POST", "/auth/otp/request", {"mobile": mobile})
    tok = call("POST", "/auth/otp/verify", {"requestId": r["requestId"], "otp": "123456"})["tokens"]["accessToken"]
    call("POST", "/onboarding/role", {"role": role}, tok)
    me = call("GET", "/me", token=tok)
    if me["pendingPolicies"]:
        call("POST", "/me/policies/accept", {"policyDocumentIds": [p["id"] for p in me["pendingPolicies"]]}, tok)
    call("POST", "/verification/mock/complete", {"outcome": "adult", "nativeDistrictId": 26}, tok)
    me = call("POST", "/onboarding/profile",
              {"displayName": name, "sportId": 1, "city": "Chennai", "state": "Tamil Nadu"}, tok)
    return tok, me


def unread(tok):
    return call("GET", "/notifications/unread-count", token=tok)["unreadCount"]


# --- onboarding: player and coach ---
call("GET", "/health")
pName, cName = f"Player{RUN}", f"Coach{RUN}"
pTok, player = onboard("8" + RUN + "201", "player", pName)
check(player["onboardingStep"] == "done" and "player" in player["roles"], "player onboarded", player)
cTok, coach = onboard("8" + RUN + "202", "coach", cName)
check(coach["onboardingStep"] == "done" and "coach" in coach["roles"], "coach onboarded", coach)
pid, cid = player["id"], coach["id"]

# --- profile with sports ---
prof = call("PUT", "/me/profile", {"bio": "Opening batter", "sports": [{"sportId": 1, "isPrimary": True}]}, pTok)
check(prof["bio"] == "Opening batter" and any(s["sport"]["id"] == 1 for s in prof["sports"]),
      "player profile + sport saved", prof)
seen = call("GET", f"/profiles/{pid}", token=cTok)
check(seen["user"]["id"] == pid and not seen["isOwn"], "coach can view player profile", seen)

# --- search people ---
res = items(call("GET", "/search/people?" + urllib.parse.urlencode({"q": cName, "role": "coach"}), token=pTok))
check(any(u["id"] == cid for u in res), "people search finds the coach by name + role", res)
glob = call("GET", "/search?" + urllib.parse.urlencode({"q": pName}), token=cTok)
check(any(u["id"] == pid for u in glob["people"]), "global search finds the player", glob)

# --- follow ---
before = unread(cTok)
f = call("POST", f"/users/{cid}/follow", token=pTok)
check(f["following"] is True and f["followersCount"] >= 1, "player follows coach", f)
followers = items(call("GET", f"/users/{cid}/followers", token=cTok))
check(any(u["id"] == pid for u in followers), "coach's followers list has the player", followers)
eventually(lambda: (unread(cTok) > before, unread(cTok)), "coach notified of the new follower")

# --- posts, feed, like, comment ---
post = call("POST", "/posts", {"body": f"Nets session today {RUN}"}, cTok)
check(post["id"] > 0, "coach creates a post", post)
fol = items(call("GET", "/feed?mode=following", token=pTok))
check(any(x["id"] == post["id"] for x in fol), "post is in the player's Following feed", fol)
disc = items(call("GET", "/feed?mode=discover", token=pTok))
check(any(x["id"] == post["id"] for x in disc), "post is in the Discover feed", disc)
before = unread(cTok)
like = call("POST", f"/posts/{post['id']}/like", token=pTok)
check(like["liked"] is True and like["likeCount"] == 1, "player likes the post", like)
com = call("POST", f"/posts/{post['id']}/comments", {"body": "Great work"}, pTok)
check(com["id"] > 0, "player comments on the post", com)
comments = items(call("GET", f"/posts/{post['id']}/comments", token=cTok))
check(any(x["id"] == com["id"] for x in comments), "coach sees the comment", comments)
eventually(lambda: (unread(cTok) >= before + 2, unread(cTok)), "coach notified of like + comment")
call("DELETE", f"/posts/{post['id']}", token=pTok, expect=(403, 404))
check(True, "player cannot delete the coach's post (403/404)")

# --- direct messages ---
conv = call("POST", "/chat/direct", {"userId": cid}, pTok)
check(conv["id"] > 0, "player opens a DM with the coach", conv)
msg = call("POST", f"/chat/conversations/{conv['id']}/messages",
           {"clientMsgId": str(uuid.uuid4()), "kind": "text", "body": "Hi coach"}, pTok, expect=(200, 201))
check(msg["id"] > 0, "player sends a message", msg)
convs = items(call("GET", "/chat/conversations", token=cTok))
check(any(c["id"] == conv["id"] for c in convs), "coach sees the conversation", convs)
msgs = items(call("GET", f"/chat/conversations/{conv['id']}/messages", token=cTok))
check(any(m["id"] == msg["id"] for m in msgs), "coach reads the message", msgs)
uc = call("GET", "/chat/unread-count", token=cTok)
check(uc.get("messages", 0) >= 1, "coach's unread message count >= 1", uc)

# --- notifications list + mark read ---
page = call("GET", "/notifications", token=cTok)
check(len(items(page)) >= 3, "coach's notifications list has follow/like/comment", page)
call("POST", "/notifications/read", {"all": True}, cTok, expect=(200, 204))
check(unread(cTok) == 0, "mark all read -> unread 0")

# --- auth ---
call("GET", "/feed?mode=discover", expect=401)
check(True, "feed without a token -> 401")

# --- account deletion (docs/security.md): sessions end, messages hidden, posts hidden ---
call("POST", "/notifications/devices", {"platform": "android", "token": f"smoke-device-{RUN}"}, pTok, expect=(200, 204))
pPost = call("POST", "/posts", {"body": f"Player post {RUN}"}, pTok)
call("DELETE", "/me", token=pTok, expect=(200, 204))
call("GET", "/me", token=pTok, expect=401)
check(True, "deleted account's token no longer works (401)")


def player_message_hidden():
    ms = items(call("GET", f"/chat/conversations/{conv['id']}/messages", token=cTok))
    mine = [m for m in ms if m["id"] == msg["id"]]
    return (not mine or (mine[0]["status"] == "deleted" and mine[0].get("body") is None)), mine


eventually(player_message_hidden, "deleted user's chat message is hidden from the coach")
disc = items(call("GET", "/feed?mode=discover", token=cTok))
check(all(x["id"] != pPost["id"] for x in disc), "deleted user's post is gone from the feed", disc)
print(f"(device token smoke-device-{RUN} should now be gone from device_tokens)")

print(f"\nALL {ok_count} CHECKS PASSED")
