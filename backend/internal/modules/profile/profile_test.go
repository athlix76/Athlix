package profile

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/modules/user/usertest"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	pmedia "athlix/backend/internal/platform/media"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

var errDup = errors.New("Error 1062 (23000): Duplicate entry '7-1-2025' for key 'uq'")

func tUser(id int64, role string) userdir.Basic {
	u := testutil.VerifiedUser(id)
	u.DisplayName, u.PrimaryRole, u.Roles = "Ravi", role, []string{role}
	return u
}

// ---- setup ----

func setupIn() map[string]any {
	return map[string]any{"displayName": "Asha Patil", "sportId": 1, "positionId": 2, "city": "Pune", "district": "Pune",
		"state": "Maharashtra", "bio": "Opening bat", "academyClub": "PYC", "photoUploadId": 70}
}

func (e *env) expectSetupStart(step string) {
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`SELECT onboarding_step FROM users WHERE id = ? AND status <> 'deleted' FOR UPDATE`)).WithArgs(me).
		WillReturnRows(sqlmock.NewRows([]string{"onboarding_step"}).AddRow(step))
}

func (e *env) expectDOB(t *testing.T, dob string) {
	t.Helper()
	rows := sqlmock.NewRows([]string{"dob_enc"})
	if dob != "" {
		enc, err := e.d.Crypto.EncryptString(dob)
		if err != nil {
			t.Fatal(err)
		}
		rows.AddRow(enc)
	}
	e.mock.ExpectQuery(q(`SELECT dob_enc FROM identity_verifications WHERE user_id = ? AND status = 'verified'`)).WithArgs(me).WillReturnRows(rows)
}

func TestSetupHappyPath(t *testing.T) {
	u := adult()
	u.OnboardingStep = "profile"
	e := newEnv(t, nil, u)
	e.expectSetupStart("profile")
	e.expectActiveSport(1, true)
	e.expectActivePosition(1, 2, true)
	e.expectDOB(t, "2001-03-04")
	e.expectClaim(70, me, pmedia.PurposeProfilePhoto)
	e.mock.ExpectExec(q(`INSERT INTO profiles (user_id, bio, sport_id, position_id, city, district, state, academy_club, photo_upload_id, birth_year, visibility, stats_visibility)`)).
		WithArgs(me, "Opening bat", int64(1), int64(2), "Pune", "Pune", "Maharashtra", "PYC", int64(70), int64(2001), "public", "public").
		WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`INSERT INTO profile_sports (user_id, sport_id, position_id, is_primary)`)).WithArgs(me, int64(1), int64(2), true).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`UPDATE users SET display_name = ? WHERE id = ?`)).WithArgs("Asha Patil", me).WillReturnResult(sqlmock.NewResult(0, 1))
	usertest.ExpectAdvance(e.mock, me, usertest.AdvanceRow{PrimaryRole: "player", Step: "profile", Verify: "verified", Guardian: "not_required", Profiles: 1, Roles: []string{"player"}}, "done")
	e.mock.ExpectCommit()
	usertest.ExpectMe(e.mock, usertest.MeRow{ID: me, DisplayName: "Asha Patil", PrimaryRole: "player", Roles: []string{"player"}, Step: "done"})

	b := status(t, e.do(http.MethodPost, "/onboarding/profile", setupIn()), http.StatusOK)
	var m struct {
		OnboardingStep string `json:"onboardingStep"`
	}
	_ = json.Unmarshal(b.Data, &m)
	if m.OnboardingStep != "done" {
		t.Fatalf("me = %s", b.Data)
	}
}

func TestSetupMinorDefaultVisibilityFromConfig(t *testing.T) {
	u := minor()
	u.OnboardingStep = "profile"
	e := newEnv(t, map[string]any{"minor.profile_default_visibility": "private"}, u)
	e.expectSetupStart("profile")
	e.expectActiveSport(1, true)
	e.expectDOB(t, "") // no verified DOB: birth_year stays NULL
	e.mock.ExpectExec(q(`INSERT INTO profiles`)).
		WithArgs(me, nil, int64(1), nil, nil, nil, nil, nil, nil, nil, "private", "private").WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`INSERT INTO profile_sports`)).WithArgs(me, int64(1), nil, true).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`UPDATE users SET display_name = ?`)).WithArgs("Asha", me).WillReturnResult(sqlmock.NewResult(0, 1))
	usertest.ExpectAdvance(e.mock, me, usertest.AdvanceRow{PrimaryRole: "player", Step: "profile", Verify: "verified", IsMinor: true, Guardian: "granted", Profiles: 1, Roles: []string{"player"}}, "done")
	e.mock.ExpectCommit()
	usertest.ExpectMe(e.mock, usertest.MeRow{ID: me, PrimaryRole: "player", Roles: []string{"player"}, IsMinor: true, Guardian: "granted"})
	status(t, e.do(http.MethodPost, "/onboarding/profile", map[string]any{"displayName": "Asha", "sportId": 1}), http.StatusOK)
}

func TestSetupValidation(t *testing.T) {
	e := newEnv(t, nil, adult())
	cases := map[string]map[string]any{
		"displayName":   {"displayName": "A", "sportId": 1},
		"sportId":       {"displayName": "Asha"},
		"bio":           {"displayName": "Asha", "sportId": 1, "bio": strings.Repeat("b", 501)},
		"city":          {"displayName": "Asha", "sportId": 1, "city": strings.Repeat("c", 65)},
		"academyClub":   {"displayName": "Asha", "sportId": 1, "academyClub": strings.Repeat("a", 121)},
		"photoUploadId": {"displayName": "Asha", "sportId": 1, "photoUploadId": -1},
	}
	for field, in := range cases {
		w := e.do(http.MethodPost, "/onboarding/profile", in)
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields[field] == "" {
			t.Errorf("%s: %d %s", field, w.Code, w.Body)
		}
	}
	w := e.do(http.MethodPost, "/onboarding/profile", map[string]any{"displayName": "Asha\x07", "sportId": 1})
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("control char: %d", w.Code)
	}
}

func TestSetupCatalogueAndState(t *testing.T) {
	t.Run("inactive sport", func(t *testing.T) {
		e := newEnv(t, nil, adult())
		e.expectSetupStart("profile")
		e.expectActiveSport(1, false)
		e.mock.ExpectRollback()
		w := e.do(http.MethodPost, "/onboarding/profile", map[string]any{"displayName": "Asha", "sportId": 1})
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["sportId"] == "" {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	})
	t.Run("position of another sport", func(t *testing.T) {
		e := newEnv(t, nil, adult())
		e.expectSetupStart("profile")
		e.expectActiveSport(1, true)
		e.expectActivePosition(1, 9, false)
		e.mock.ExpectRollback()
		w := e.do(http.MethodPost, "/onboarding/profile", map[string]any{"displayName": "Asha", "sportId": 1, "positionId": 9})
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["positionId"] == "" {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	})
	for _, step := range []string{"done", "verification"} {
		t.Run("step "+step, func(t *testing.T) {
			e := newEnv(t, nil, adult())
			e.expectSetupStart(step)
			e.mock.ExpectRollback()
			w := e.do(http.MethodPost, "/onboarding/profile", map[string]any{"displayName": "Asha", "sportId": 1})
			if w.Code != http.StatusConflict || decode(t, w).Error.Code != "INVALID_STATE" {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
	t.Run("photo not the caller's", func(t *testing.T) {
		e := newEnv(t, nil, adult())
		e.expectSetupStart("profile")
		e.expectActiveSport(1, true)
		e.expectDOB(t, "")
		e.expectUpload(70, me, "", "") // owner-scoped lookup finds nothing
		e.mock.ExpectRollback()
		w := e.do(http.MethodPost, "/onboarding/profile", map[string]any{"displayName": "Asha", "sportId": 1, "photoUploadId": 70})
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["photoUploadId"] == "" {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	})
	t.Run("photo of the wrong purpose", func(t *testing.T) {
		e := newEnv(t, nil, adult())
		e.expectSetupStart("profile")
		e.expectActiveSport(1, true)
		e.expectDOB(t, "")
		e.expectUpload(70, me, pmedia.PurposeChatImage, "confirmed")
		e.mock.ExpectRollback()
		if w := e.do(http.MethodPost, "/onboarding/profile", map[string]any{"displayName": "Asha", "sportId": 1, "photoUploadId": 70}); w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	})
}

// ---- view ----

type viewOut struct {
	User struct {
		ID   int64   `json:"id"`
		City *string `json:"city"`
	} `json:"user"`
	IsOwn        bool           `json:"isOwn"`
	Restricted   bool           `json:"restricted"`
	Bio          *string        `json:"bio"`
	Sports       []ProfileSport `json:"sports"`
	District     *string        `json:"district"`
	BirthYear    *int           `json:"birthYear"`
	Visibility   *string        `json:"visibility"`
	Followers    int            `json:"followersCount"`
	IsFollowing  bool           `json:"isFollowing"`
	IsFollowedBy bool           `json:"isFollowedBy"`
	CanMessage   bool           `json:"canMessage"`
	Badges       []string       `json:"badges"`
	Coaching     *int           `json:"coachingSinceYear"`
	Consultation *Consultation  `json:"consultation"`
	Organiser    *OrganiserInfo `json:"organiser"`
}

func viewOf(t *testing.T, b body) viewOut {
	t.Helper()
	var v viewOut
	if err := json.Unmarshal(b.Data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestViewOtherCoach(t *testing.T) {
	e := newEnv(t, nil, adult())
	tg := tUser(other, "coach")
	e.expectBasic(tg)
	e.expectBlocked(me, other, false)
	e.expectProfile(other, &prow{Bio: "Coach", District: "Pune", BirthYear: 1990, CoachingSince: 2015, Followers: 3}, false)
	e.expectFollows(me, other, false)
	e.expectFollows(other, me, true)
	e.expectSummary(other, "Ravi", "Pune")
	e.expectView(other, me)
	e.expectSports(other)
	e.expectBenefits(other, "verified_coach_badge")
	e.mock.ExpectQuery(q(`SELECT offered FROM coach_consultations WHERE user_id = ?`)).WithArgs(other).WillReturnRows(sqlmock.NewRows([]string{"offered"}).AddRow(true))
	e.expectPrivacy(other, "followers") // target follows the viewer → may message

	v := viewOf(t, status(t, e.do(http.MethodGet, "/profiles/9", nil), http.StatusOK))
	if v.IsOwn || v.Restricted || v.Bio == nil || *v.Bio != "Coach" || v.District == nil || v.Followers != 3 {
		t.Fatalf("view = %+v", v)
	}
	if v.BirthYear != nil {
		t.Fatal("birthYear is shown only to the owner")
	}
	if v.User.City == nil || *v.User.City != "Pune" {
		t.Fatal("adult city is public")
	}
	if len(v.Sports) != 2 || !v.Sports[0].IsPrimary || v.Sports[0].Position == nil || v.Sports[1].Position != nil {
		t.Fatalf("sports = %+v", v.Sports)
	}
	if len(v.Badges) != 1 || v.Badges[0] != "coach_pro" || v.Consultation == nil || !v.Consultation.Offered || v.Coaching == nil || *v.Coaching != 2015 {
		t.Fatalf("coach parts = %+v", v)
	}
	if v.IsFollowing || !v.IsFollowedBy || !v.CanMessage || v.Organiser != nil {
		t.Fatalf("relations = %+v", v)
	}
}

func TestViewOrganiser(t *testing.T) {
	e := newEnv(t, nil, adult())
	tg := tUser(other, "organiser")
	e.expectBasic(tg)
	e.expectBlocked(me, other, false)
	e.expectProfile(other, &prow{}, false)
	e.expectFollows(me, other, false)
	e.expectFollows(other, me, false)
	e.expectSummary(other, "Ravi", nil)
	e.expectView(other, me)
	e.expectSports(other)
	e.expectBenefits(other)
	e.mock.ExpectQuery(q(`SELECT status, org_name, org_type FROM organiser_profiles WHERE user_id = ?`)).WithArgs(other).
		WillReturnRows(sqlmock.NewRows([]string{"status", "org_name", "org_type"}).AddRow("approved", "Pune League", "league"))
	e.expectPrivacy(other, "everyone")
	v := viewOf(t, status(t, e.do(http.MethodGet, "/profiles/9", nil), http.StatusOK))
	if v.Organiser == nil || v.Organiser.Status != "approved" || *v.Organiser.OrgName != "Pune League" || !v.CanMessage || v.Consultation != nil {
		t.Fatalf("view = %+v", v)
	}
}

func TestViewOwn(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.expectBasic(adult())
	e.expectProfile(me, &prow{BirthYear: 2001, Visibility: "private"}, false)
	e.expectSummary(me, "Asha", "Pune")
	e.expectSports(me)
	e.expectBenefits(me)
	// No block / follow checks, no view recorded, no canMessage.
	v := viewOf(t, status(t, e.do(http.MethodGet, "/profiles/7", nil), http.StatusOK))
	if !v.IsOwn || v.Restricted || v.BirthYear == nil || *v.BirthYear != 2001 || v.CanMessage {
		t.Fatalf("own = %+v", v)
	}
}

func TestViewNotFoundCases(t *testing.T) {
	cases := map[string]func(e *env){
		"missing user": func(e *env) { e.expectBasicMissing(other) },
		"deleted": func(e *env) {
			u := tUser(other, "player")
			u.Status = "deleted"
			e.expectBasic(u)
		},
		"suspended": func(e *env) {
			u := tUser(other, "player")
			u.Status = "suspended"
			e.expectBasic(u)
		},
		"onboarding not done": func(e *env) {
			u := tUser(other, "player")
			u.OnboardingStep = "profile"
			e.expectBasic(u)
		},
		"blocked either way": func(e *env) {
			e.expectBasic(tUser(other, "player"))
			e.expectBlocked(me, other, true)
		},
		"no profile row": func(e *env) {
			e.expectBasic(tUser(other, "player"))
			e.expectBlocked(me, other, false)
			e.expectProfile(other, nil, false)
		},
	}
	for name, setup := range cases {
		for _, path := range []string{"/profiles/9", "/profiles/9/stats", "/profiles/9/achievements"} {
			t.Run(name+" "+path, func(t *testing.T) {
				e := newEnv(t, nil, adult())
				setup(e)
				w := e.do(http.MethodGet, path, nil)
				if w.Code != http.StatusNotFound || decode(t, w).Error.Code != "NOT_FOUND" {
					t.Fatalf("%d %s", w.Code, w.Body)
				}
			})
		}
	}
	e := newEnv(t, nil, adult())
	if w := e.do(http.MethodGet, "/profiles/abc", nil); w.Code != http.StatusNotFound && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad id: %d", w.Code)
	}
}

func TestViewRestrictedFollowersOnly(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.expectBasic(tUser(other, "player"))
	e.expectBlocked(me, other, false)
	e.expectProfile(other, &prow{Bio: "secret", Visibility: "followers"}, false)
	e.expectFollows(me, other, false)
	e.expectFollows(other, me, false)
	e.expectSummary(other, "Ravi", "Pune")
	e.expectView(other, me)
	v := viewOf(t, status(t, e.do(http.MethodGet, "/profiles/9", nil), http.StatusOK))
	if !v.Restricted || v.Bio != nil || v.Visibility != nil || len(v.Sports) != 0 || v.CanMessage {
		t.Fatalf("restricted = %+v", v)
	}

	// A follower sees the full profile.
	e2 := newEnv(t, nil, adult())
	e2.expectBasic(tUser(other, "player"))
	e2.expectBlocked(me, other, false)
	e2.expectProfile(other, &prow{Bio: "secret", Visibility: "followers"}, false)
	e2.expectFollows(me, other, true)
	e2.expectFollows(other, me, false)
	e2.expectSummary(other, "Ravi", "Pune")
	e2.expectView(other, me)
	e2.expectSports(other)
	e2.expectBenefits(other)
	e2.expectPrivacy(other, "nobody")
	e2.expectConsultation(me, other, false)
	v = viewOf(t, status(t, e2.do(http.MethodGet, "/profiles/9", nil), http.StatusOK))
	if v.Restricted || v.Bio == nil || !v.IsFollowing || v.CanMessage {
		t.Fatalf("follower view = %+v", v)
	}
}

func TestViewMinorTargetHidesCityAndMinorViewerRecordsNoView(t *testing.T) {
	e := newEnv(t, nil, adult())
	tg := tUser(other, "player")
	tg.IsMinor = true
	e.expectBasic(tg)
	e.expectBlocked(me, other, false)
	e.expectProfile(other, &prow{}, false)
	e.expectFollows(me, other, false)
	e.expectFollows(other, me, false)
	e.expectSummary(other, "Kid", "Pune")
	e.expectView(other, me)
	e.expectSports(other)
	e.expectBenefits(other)
	e.expectPrivacy(other, "everyone") // minor.dm_policy mutual_follow: not mutual → no DMs
	v := viewOf(t, status(t, e.do(http.MethodGet, "/profiles/9", nil), http.StatusOK))
	if v.User.City != nil || v.CanMessage {
		t.Fatalf("minor target = %+v", v)
	}

	e2 := newEnv(t, nil, minor())
	e2.expectBasic(tUser(other, "player"))
	e2.expectBlocked(me, other, false)
	e2.expectProfile(other, &prow{}, false)
	e2.expectFollows(me, other, false)
	e2.expectFollows(other, me, false)
	e2.expectSummary(other, "Ravi", "Pune")
	// no INSERT INTO profile_views for a minor viewer
	e2.expectSports(other)
	e2.expectBenefits(other)
	e2.expectPrivacy(other, "everyone")
	status(t, e2.do(http.MethodGet, "/profiles/9", nil), http.StatusOK)
}

func TestViewRecordFailureIsNotFatal(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.expectBasic(tUser(other, "player"))
	e.expectBlocked(me, other, false)
	e.expectProfile(other, &prow{Visibility: "private"}, false)
	e.expectFollows(me, other, false)
	e.expectFollows(other, me, false)
	e.expectSummary(other, "Ravi", nil)
	e.mock.ExpectExec(q(`INSERT IGNORE INTO profile_views`)).WillReturnError(errors.New("db down"))
	v := viewOf(t, status(t, e.do(http.MethodGet, "/profiles/9", nil), http.StatusOK))
	if !v.Restricted {
		t.Fatal("private profile is restricted")
	}
}

func TestCanMessageRules(t *testing.T) {
	type tc struct {
		name                  string
		cfg                   map[string]any
		viewerMinor, tgtMinor bool
		viewerStatus          string
		allow                 string // "" = no settings query expected
		following, followedBy bool
		consult               *bool // nil = no consultation query expected
		want                  bool
	}
	yes, no := true, false
	cases := []tc{
		{name: "viewer suspended", viewerStatus: "suspended", want: false},
		{name: "everyone", allow: "everyone", want: true},
		{name: "everyone but follow required", cfg: map[string]any{"chat.dm_requires_follow": true}, allow: "everyone", consult: &no, want: false},
		{name: "follow required and followed", cfg: map[string]any{"chat.dm_requires_follow": true}, allow: "everyone", followedBy: true, want: true},
		{name: "followers, target follows viewer", allow: "followers", followedBy: true, want: true},
		{name: "followers, only viewer follows", allow: "followers", following: true, consult: &no, want: false},
		{name: "mutual, one way, accepted consultation", allow: "mutual", followedBy: true, consult: &yes, want: true},
		{name: "mutual both ways", allow: "mutual", following: true, followedBy: true, want: true},
		{name: "nobody", allow: "nobody", following: true, followedBy: true, consult: &no, want: false},
		{name: "nobody but consultation", allow: "nobody", consult: &yes, want: true},
		{name: "minor target, mutual_follow, mutual", tgtMinor: true, allow: "everyone", following: true, followedBy: true, want: true},
		{name: "minor target, mutual_follow, one way", tgtMinor: true, allow: "everyone", followedBy: true, want: false},
		{name: "minor viewer, mutual_follow, mutual but target nobody", viewerMinor: true, allow: "nobody", following: true, followedBy: true, want: false},
		{name: "minor, policy anyone", cfg: map[string]any{"minor.dm_policy": "anyone"}, tgtMinor: true, allow: "everyone", want: true},
		{name: "minor, policy none", cfg: map[string]any{"minor.dm_policy": "none"}, viewerMinor: true, allow: "everyone", following: true, followedBy: true, want: false},
		{name: "minor, unknown policy is mutual_follow", cfg: map[string]any{"minor.dm_policy": "weird"}, tgtMinor: true, allow: "everyone", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := adult()
			v.IsMinor = c.viewerMinor
			if c.viewerStatus != "" {
				v.Status = c.viewerStatus
			}
			e := newEnv(t, c.cfg, v)
			tg := tUser(other, "player")
			tg.IsMinor = c.tgtMinor
			if c.allow != "" {
				e.expectPrivacy(other, c.allow)
			}
			if c.consult != nil {
				e.expectConsultation(me, other, *c.consult)
			}
			got, err := e.m.svc.canMessage(context.Background(), v, subject{target: tg, following: c.following, followedBy: c.followedBy})
			if err != nil || got != c.want {
				t.Fatalf("canMessage = %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

// ---- update ----

func TestUpdateHappyPath(t *testing.T) {
	coach := tUser(me, "coach")
	e := newEnv(t, nil, coach)
	e.storagePut("profile_photo/7/old.jpg")
	in := map[string]any{
		"displayName": "Ravi K", "bio": "", "city": "Mumbai", "visibility": "followers", "coachingSinceYear": 2010,
		"sports":        []map[string]any{{"sportId": 3}, {"sportId": 1, "positionId": 2, "isPrimary": true}},
		"photoUploadId": 71,
	}
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{Photo: 70}, true)
	e.expectActiveSport(3, true)
	e.expectActiveSport(1, true)
	e.expectActivePosition(1, 2, true)
	e.expectClaim(71, me, pmedia.PurposeProfilePhoto)
	e.mock.ExpectExec(q(`UPDATE profiles SET bio = ?, city = ?, visibility = ?, coaching_since_year = ?, sport_id = ?, position_id = ?, photo_upload_id = ? WHERE user_id = ?`)).
		WithArgs(nil, "Mumbai", "followers", 2010, int64(1), int64(2), int64(71), me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`DELETE FROM profile_sports WHERE user_id = ?`)).WithArgs(me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`INSERT INTO profile_sports`)).WithArgs(me, int64(3), nil, false).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`INSERT INTO profile_sports`)).WithArgs(me, int64(1), int64(2), true).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`UPDATE users SET display_name = ? WHERE id = ?`)).WithArgs("Ravi K", me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.expectPurge(70, "profile_photo/7/old.jpg")
	// The response is the caller's own profile.
	e.expectBasic(coach)
	e.expectProfile(me, &prow{City: "Mumbai", Visibility: "followers", CoachingSince: 2010, Photo: 71}, false)
	e.expectSummary(me, "Ravi K", "Mumbai")
	e.expectSports(me)
	e.expectBenefits(me)
	e.mock.ExpectQuery(q(`SELECT offered FROM coach_consultations`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"offered"}))

	v := viewOf(t, status(t, e.do(http.MethodPut, "/me/profile", in), http.StatusOK))
	if !v.IsOwn || v.Coaching == nil || *v.Coaching != 2010 || v.Consultation == nil || v.Consultation.Offered {
		t.Fatalf("view = %+v", v)
	}
	if e.storageHas("profile_photo/7/old.jpg") {
		t.Fatal("old photo file must be deleted")
	}
}

func TestUpdateClearPhotoAndNoop(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{Photo: 70}, true)
	e.mock.ExpectExec(q(`UPDATE profiles SET photo_upload_id = ? WHERE user_id = ?`)).WithArgs(nil, me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.expectPurge(70, "k")
	e.expectBasic(adult())
	e.expectProfile(me, &prow{}, false)
	e.expectSummary(me, "Asha", nil)
	e.expectSports(me)
	e.expectBenefits(me)
	status(t, e.do(http.MethodPut, "/me/profile", map[string]any{"photoUploadId": 0}), http.StatusOK)

	// The same photo again claims and purges nothing.
	e2 := newEnv(t, nil, adult())
	e2.mock.ExpectBegin()
	e2.expectProfile(me, &prow{Photo: 70}, true)
	e2.mock.ExpectCommit()
	e2.expectBasic(adult())
	e2.expectProfile(me, &prow{Photo: 70}, false)
	e2.expectSummary(me, "Asha", nil)
	e2.expectSports(me)
	e2.expectBenefits(me)
	status(t, e2.do(http.MethodPut, "/me/profile", map[string]any{"photoUploadId": 70}), http.StatusOK)
}

func TestUpdateValidation(t *testing.T) {
	e := newEnv(t, nil, adult())
	cases := map[string]map[string]any{
		"displayName":       {"displayName": "x"},
		"bio":               {"bio": strings.Repeat("b", 501)},
		"visibility":        {"visibility": "secret"},
		"statsVisibility":   {"statsVisibility": "everyone"},
		"coachingSinceYear": {"coachingSinceYear": 2010}, // the caller is not a coach
		"photoUploadId":     {"photoUploadId": -1},
	}
	for field, in := range cases {
		w := e.do(http.MethodPut, "/me/profile", in)
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields[field] == "" {
			t.Errorf("%s: %d %s", field, w.Code, w.Body)
		}
	}
	for name, sports := range map[string][]map[string]any{
		"duplicate":   {{"sportId": 1}, {"sportId": 1}},
		"two primary": {{"sportId": 1, "isPrimary": true}, {"sportId": 2, "isPrimary": true}},
		"empty":       {},
		"zero id":     {{"sportId": 0}},
	} {
		w := e.do(http.MethodPut, "/me/profile", map[string]any{"sports": sports})
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["sports"] == "" {
			t.Errorf("sports %s: %d %s", name, w.Code, w.Body)
		}
	}

	coach := newEnv(t, nil, tUser(me, "coach"))
	for _, y := range []int{1899, 2027} {
		w := coach.do(http.MethodPut, "/me/profile", map[string]any{"coachingSinceYear": y})
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["coachingSinceYear"] == "" {
			t.Errorf("year %d: %d", y, w.Code)
		}
	}
}

func TestUpdateMinorVisibilityRule(t *testing.T) {
	e := newEnv(t, nil, minor())
	for _, f := range []string{"visibility", "statsVisibility"} {
		w := e.do(http.MethodPut, "/me/profile", map[string]any{f: "public"})
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields[f] == "" {
			t.Errorf("%s: %d %s", f, w.Code, w.Body)
		}
	}
	// When the configured minor default is public, a minor may choose public.
	e2 := newEnv(t, map[string]any{"minor.profile_default_visibility": "public"}, minor())
	e2.mock.ExpectBegin()
	e2.expectProfile(me, &prow{Visibility: "followers"}, true)
	e2.mock.ExpectExec(q(`UPDATE profiles SET visibility = ? WHERE user_id = ?`)).WithArgs("public", me).WillReturnResult(sqlmock.NewResult(0, 1))
	e2.mock.ExpectCommit()
	m := minor()
	e2.expectBasic(m)
	e2.expectProfile(me, &prow{}, false)
	e2.expectSummary(me, "Kid", "Pune")
	e2.expectSports(me)
	e2.expectBenefits(me)
	status(t, e2.do(http.MethodPut, "/me/profile", map[string]any{"visibility": "public"}), http.StatusOK)
}

func TestUpdateInactiveSportAndMissingProfile(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{}, true)
	e.expectActiveSport(5, false)
	e.mock.ExpectRollback()
	w := e.do(http.MethodPut, "/me/profile", map[string]any{"sports": []map[string]any{{"sportId": 5}}})
	if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["sports"] == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	e.mock.ExpectBegin()
	e.expectProfile(me, nil, true)
	e.mock.ExpectRollback()
	if w := e.do(http.MethodPut, "/me/profile", map[string]any{"bio": "x"}); w.Code != http.StatusNotFound {
		t.Fatalf("no profile: %d", w.Code)
	}
}

// ---- profile viewers ----

func TestViewersPlanRequired(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.expectBenefits(me)
	w := e.do(http.MethodGet, "/me/profile-viewers", nil)
	b := status(t, w, http.StatusForbidden)
	if b.Error.Code != httpx.CodePlanRequired || b.Error.Fields["benefit"] != "see_profile_viewers" {
		t.Fatalf("%s", w.Body)
	}
}

const viewersSQL = `SELECT v.viewer_id, MAX(v.viewed_on) AS last_on FROM profile_views v`

func viewerRows(rows ...[2]any) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"viewer_id", "last_on"})
	for _, x := range rows {
		d, _ := time.Parse(dateLayout, x[1].(string))
		r.AddRow(x[0], d)
	}
	return r
}

func TestViewersWindowCursorAndMinorCity(t *testing.T) {
	e := newEnv(t, map[string]any{"profile.viewers_window_days": 7}, adult())
	e.expectBenefits(me, "see_profile_viewers")
	e.mock.ExpectQuery(q(viewersSQL)).WithArgs(me, "2026-06-09", 2).
		WillReturnRows(viewerRows([2]any{other, "2026-06-14"}, [2]any{int64(11), "2026-06-12"}))
	e.expectSummary(other, "Kid", "Pune")
	e.mock.ExpectQuery(q(`SELECT u.id FROM users u WHERE u.is_minor = 1 AND u.id IN (?) AND NOT EXISTS`)).WithArgs(other, me).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(other))
	w := e.do(http.MethodGet, "/me/profile-viewers?limit=1", nil)
	b := status(t, w, http.StatusOK)
	var list []struct {
		User struct {
			ID   int64   `json:"id"`
			City *string `json:"city"`
		} `json:"user"`
		ViewedOn string `json:"viewedOn"`
	}
	_ = json.Unmarshal(b.Data, &list)
	if len(list) != 1 || list[0].User.ID != other || list[0].ViewedOn != "2026-06-14" || list[0].User.City != nil {
		t.Fatalf("viewers = %s", b.Data)
	}
	next, _ := b.Meta["nextCursor"].(string)
	if next != httpx.EncodeCursor(httpx.Cursor{ID: other, Time: "2026-06-14"}) {
		t.Fatalf("next = %q", next)
	}

	// The next page continues after the cursor; the default window is 30 days.
	e2 := newEnv(t, nil, adult())
	e2.expectBenefits(me, "see_profile_viewers")
	e2.mock.ExpectQuery(`(?s)`+q(viewersSQL)+`.*`+q(`HAVING (last_on < ? OR (last_on = ? AND v.viewer_id < ?))`)).
		WithArgs(me, "2026-05-17", "2026-06-14", "2026-06-14", other, 21).WillReturnRows(viewerRows())
	b = status(t, e2.do(http.MethodGet, "/me/profile-viewers?cursor="+next, nil), http.StatusOK)
	if string(b.Data) != "[]" || b.Meta["nextCursor"] != nil {
		t.Fatalf("page 2 = %s %v", b.Data, b.Meta)
	}

	// A cursor whose time is not a day is refused.
	e3 := newEnv(t, nil, adult())
	e3.expectBenefits(me, "see_profile_viewers")
	bad := httpx.EncodeCursor(httpx.Cursor{ID: 1, Time: "yesterday"})
	if w := e3.do(http.MethodGet, "/me/profile-viewers?cursor="+bad, nil); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad cursor: %d", w.Code)
	}
}

// ---- stats ----

const cricketFields = `[{"key":"runs","label":"Runs","type":"int"},{"key":"avg","label":"Average","type":"decimal"},{"key":"sr","label":"Strike rate","type":"percent"},{"key":"time","label":"Time","type":"duration_ms"},{"key":"note","label":"Note","type":"text"}]`

func (e *env) expectTemplate(id, sportID int64, active bool, fields string) {
	query := q(`SELECT fields FROM stat_templates WHERE id = ? AND sport_id = ?`)
	if active {
		query += q(` AND active = 1`)
	} else {
		query += `$`
	}
	rows := sqlmock.NewRows([]string{"fields"})
	if fields != "" {
		rows.AddRow([]byte(fields))
	}
	e.mock.ExpectQuery(query).WithArgs(id, sportID).WillReturnRows(rows)
}

func TestCreateStat(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.expectTemplate(4, 1, true, cricketFields)
	e.mock.ExpectExec(q(`INSERT INTO user_stats (user_id, sport_id, template_id, season, stats) VALUES (?, ?, ?, ?, ?)`)).
		WithArgs(me, int64(1), int64(4), "2025-26", anyOf(func(v driver.Value) bool {
			var m map[string]any
			return json.Unmarshal(v.([]byte), &m) == nil && m["runs"] == float64(410) && m["note"] == "ok"
		})).WillReturnResult(sqlmock.NewResult(12, 1))
	in := map[string]any{"sportId": 1, "templateId": 4, "season": "2025-26", "stats": map[string]any{"runs": 410, "avg": 41.5, "sr": 88.2, "time": 1000, "note": "ok"}}
	b := status(t, e.do(http.MethodPost, "/me/stats", in), http.StatusOK)
	var s StatEntry
	_ = json.Unmarshal(b.Data, &s)
	if s.ID != 12 || s.Season != "2025-26" || !s.UpdatedAt.Equal(fixedNow) {
		t.Fatalf("stat = %+v", s)
	}

	e.expectTemplate(4, 1, true, cricketFields)
	e.mock.ExpectExec(q(`INSERT INTO user_stats`)).WillReturnError(errDup)
	if w := e.do(http.MethodPost, "/me/stats", in); w.Code != http.StatusConflict || decode(t, w).Error.Code != "CONFLICT" {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body)
	}

	e.expectTemplate(4, 1, true, "") // inactive, or a template of another sport
	if w := e.do(http.MethodPost, "/me/stats", in); w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["templateId"] == "" {
		t.Fatalf("inactive template: %d %s", w.Code, w.Body)
	}
}

func TestCreateStatValidation(t *testing.T) {
	e := newEnv(t, nil, adult())
	for field, in := range map[string]map[string]any{
		"sportId":    {"templateId": 4, "season": "2025", "stats": map[string]any{"runs": 1}},
		"templateId": {"sportId": 1, "season": "2025", "stats": map[string]any{"runs": 1}},
		"season":     {"sportId": 1, "templateId": 4, "season": "25/26", "stats": map[string]any{"runs": 1}},
	} {
		w := e.do(http.MethodPost, "/me/stats", in)
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields[field] == "" {
			t.Errorf("%s: %d %s", field, w.Code, w.Body)
		}
	}
	for key, val := range map[string]any{
		"runs":    1.5,
		"avg":     "high",
		"sr":      101,
		"time":    -1,
		"note":    strings.Repeat("n", 101),
		"unknown": 1,
	} {
		e.expectTemplate(4, 1, true, cricketFields)
		w := e.do(http.MethodPost, "/me/stats", map[string]any{"sportId": 1, "templateId": 4, "season": "2025", "stats": map[string]any{key: val}})
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["stats."+key] == "" {
			t.Errorf("stats.%s: %d %s", key, w.Code, w.Body)
		}
	}
	e.expectTemplate(4, 1, true, cricketFields)
	w := e.do(http.MethodPost, "/me/stats", map[string]any{"sportId": 1, "templateId": 4, "season": "2025", "stats": map[string]any{}})
	if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["stats"] == "" {
		t.Errorf("empty stats: %d", w.Code)
	}
}

func TestCheckValueTypes(t *testing.T) {
	ok := map[string]any{"int": float64(3), "decimal": 2.5, "percent": float64(100), "duration_ms": float64(0), "text": "x"}
	for typ, v := range ok {
		if msg := checkValue(typ, v); msg != "" {
			t.Errorf("%s %v: %s", typ, v, msg)
		}
	}
	if checkValue("weird", float64(1)) == "" || checkValue("percent", float64(-1)) == "" || checkValue("text", 1.0) == "" {
		t.Error("bad values pass")
	}
}

func statRows(rows ...[]any) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"id", "sport_id", "template_id", "season", "stats", "updated_at"})
	for _, x := range rows {
		vals := make([]driver.Value, len(x))
		for i := range x {
			vals[i] = x[i]
		}
		r.AddRow(vals...)
	}
	return r
}

func (e *env) expectPublicTarget(visibility, statsVisibility string) {
	e.expectBasic(tUser(other, "player"))
	e.expectBlocked(me, other, false)
	e.expectProfile(other, &prow{Visibility: visibility, StatsVisibility: statsVisibility}, false)
	e.expectFollows(me, other, false)
	e.expectFollows(other, me, false)
}

func TestStatsListAndTotals(t *testing.T) {
	ts := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rows := func() *sqlmock.Rows {
		return statRows(
			[]any{int64(1), int64(1), int64(4), "2025-26", []byte(`{"runs":300,"avg":30,"note":"x"}`), ts},
			[]any{int64(2), int64(1), int64(4), "2024-25", []byte(`{"runs":110,"avg":11.5}`), ts},
		)
	}
	// With advanced_stats the owner gets career totals of numeric fields.
	e := newEnv(t, nil, adult())
	e.expectPublicTarget("public", "public")
	e.mock.ExpectQuery(q(`SELECT id, sport_id, template_id, season, stats, updated_at FROM user_stats WHERE user_id = ?`)).WithArgs(other).WillReturnRows(rows())
	e.expectBenefits(other, "advanced_stats")
	e.mock.ExpectQuery(q(`SELECT id, fields FROM stat_templates WHERE id IN (?)`)).WithArgs(int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "fields"}).AddRow(int64(4), []byte(cricketFields)))
	var s Stats
	_ = json.Unmarshal(status(t, e.do(http.MethodGet, "/profiles/9/stats", nil), http.StatusOK).Data, &s)
	if len(s.Entries) != 2 || s.Totals["runs"] != 410 || s.Totals["avg"] != 41.5 {
		t.Fatalf("stats = %+v", s)
	}
	if _, ok := s.Totals["note"]; ok {
		t.Fatal("text fields are not totalled")
	}

	// Without the benefit totals are null.
	e2 := newEnv(t, nil, adult())
	e2.expectPublicTarget("public", "public")
	e2.mock.ExpectQuery(q(`SELECT id, sport_id, template_id, season, stats, updated_at FROM user_stats`)).WithArgs(other).WillReturnRows(rows())
	e2.expectBenefits(other)
	b := status(t, e2.do(http.MethodGet, "/profiles/9/stats", nil), http.StatusOK)
	if !strings.Contains(string(b.Data), `"totals":null`) {
		t.Fatalf("stats = %s", b.Data)
	}

	// Hidden stats: no stat query at all.
	for _, vis := range [][2]string{{"public", "private"}, {"public", "followers"}, {"private", "public"}} {
		e3 := newEnv(t, nil, adult())
		e3.expectPublicTarget(vis[0], vis[1])
		b := status(t, e3.do(http.MethodGet, "/profiles/9/stats", nil), http.StatusOK)
		if string(b.Data) != `{"entries":[],"totals":null}` {
			t.Fatalf("%v: %s", vis, b.Data)
		}
	}
}

func TestUpdateAndDeleteStat(t *testing.T) {
	ts := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	e := newEnv(t, nil, adult())
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`SELECT id, sport_id, template_id, season, stats, updated_at FROM user_stats WHERE id = ? AND user_id = ? FOR UPDATE`)).WithArgs(int64(12), me).
		WillReturnRows(statRows([]any{int64(12), int64(1), int64(4), "2025", []byte(`{"runs":1}`), ts}))
	e.expectTemplate(4, 1, false, cricketFields) // a retired template still validates edits
	e.mock.ExpectExec(q(`UPDATE user_stats SET season = ?, stats = ? WHERE id = ? AND user_id = ?`)).
		WithArgs("2025", []byte(`{"runs":7}`), int64(12), me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	var s StatEntry
	_ = json.Unmarshal(status(t, e.do(http.MethodPut, "/me/stats/12", map[string]any{"stats": map[string]any{"runs": 7}}), http.StatusOK).Data, &s)
	if s.Stats["runs"] != float64(7) || s.Season != "2025" {
		t.Fatalf("stat = %+v", s)
	}

	// Another user's stat id: the scoped query finds nothing → 404.
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`FROM user_stats WHERE id = ? AND user_id = ? FOR UPDATE`)).WithArgs(int64(99), me).WillReturnRows(statRows())
	e.mock.ExpectRollback()
	if w := e.do(http.MethodPut, "/me/stats/99", map[string]any{"season": "2024"}); w.Code != http.StatusNotFound {
		t.Fatalf("other's stat: %d", w.Code)
	}
	// Season change into an existing season → 409.
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`FROM user_stats WHERE id = ? AND user_id = ? FOR UPDATE`)).WithArgs(int64(12), me).
		WillReturnRows(statRows([]any{int64(12), int64(1), int64(4), "2025", []byte(`{"runs":1}`), ts}))
	e.mock.ExpectExec(q(`UPDATE user_stats`)).WillReturnError(errDup)
	e.mock.ExpectRollback()
	if w := e.do(http.MethodPut, "/me/stats/12", map[string]any{"season": "2024"}); w.Code != http.StatusConflict {
		t.Fatalf("dup season: %d", w.Code)
	}
	if w := e.do(http.MethodPut, "/me/stats/12", map[string]any{"season": "x"}); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad season: %d", w.Code)
	}

	e.mock.ExpectExec(q(`DELETE FROM user_stats WHERE id = ? AND user_id = ?`)).WithArgs(int64(12), me).WillReturnResult(sqlmock.NewResult(0, 1))
	status(t, e.do(http.MethodDelete, "/me/stats/12", nil), http.StatusNoContent)
	e.mock.ExpectExec(q(`DELETE FROM user_stats WHERE id = ? AND user_id = ?`)).WithArgs(int64(99), me).WillReturnResult(sqlmock.NewResult(0, 0))
	if w := e.do(http.MethodDelete, "/me/stats/99", nil); w.Code != http.StatusNotFound {
		t.Fatalf("delete other's: %d", w.Code)
	}
}

// ---- achievements ----

func achRows(rows ...[]any) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"id", "sport_id", "title", "description", "level", "achieved_on", "certificate_upload_id", "created_at"})
	for _, x := range rows {
		vals := make([]driver.Value, len(x))
		for i := range x {
			vals[i] = x[i]
		}
		r.AddRow(vals...)
	}
	return r
}

var achTime = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func achRow(id int64, cert any) []any {
	return []any{id, int64(1), "District champion", "Final", "district", time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), cert, achTime}
}

func (e *env) expectAchievement(id int64, row []any, forUpdate bool) {
	query := q(`SELECT id, sport_id, title, description, level, achieved_on, certificate_upload_id, created_at FROM achievements WHERE id = ? AND user_id = ?`)
	if forUpdate {
		query += q(` FOR UPDATE`)
	} else {
		query += `$`
	}
	rows := achRows()
	if row != nil {
		rows = achRows(row)
	}
	e.mock.ExpectQuery(query).WithArgs(id, me).WillReturnRows(rows)
}

func TestCreateAchievement(t *testing.T) {
	e := newEnv(t, map[string]any{"profile.max_achievements": 3}, adult())
	in := map[string]any{"sportId": 1, "title": "District champion", "description": "Final", "level": "district", "achievedOn": "2025-12-01", "certificateUploadId": 80}
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`SELECT user_id FROM profiles WHERE user_id = ? FOR UPDATE`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(me))
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM achievements WHERE user_id = ?`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(2))
	e.expectActiveSport(1, true)
	e.expectClaim(80, me, pmedia.PurposeAchievementCert)
	e.mock.ExpectExec(q(`INSERT INTO achievements (user_id, sport_id, title, description, level, achieved_on, certificate_upload_id)`)).
		WithArgs(me, int64(1), "District champion", "Final", "district", "2025-12-01", int64(80)).WillReturnResult(sqlmock.NewResult(5, 1))
	e.mock.ExpectCommit()
	e.expectAchievement(5, achRow(5, int64(80)), false)
	e.expectViewURL(80, "achievement_certificate/7/c.pdf")
	var a Achievement
	_ = json.Unmarshal(status(t, e.do(http.MethodPost, "/me/achievements", in), http.StatusOK).Data, &a)
	if a.ID != 5 || a.CertificateURL == nil || *a.CertificateURL != "https://storage.test/get/achievement_certificate/7/c.pdf" || *a.AchievedOn != "2025-12-01" {
		t.Fatalf("achievement = %+v", a)
	}

	// The configured maximum is reached → 409 LIMIT_REACHED.
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`SELECT user_id FROM profiles WHERE user_id = ? FOR UPDATE`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(me))
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM achievements`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(3))
	e.mock.ExpectRollback()
	w := e.do(http.MethodPost, "/me/achievements", map[string]any{"title": "Another"})
	if w.Code != http.StatusConflict || decode(t, w).Error.Code != "LIMIT_REACHED" {
		t.Fatalf("limit: %d %s", w.Code, w.Body)
	}

	// No profile yet → 404.
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`SELECT user_id FROM profiles WHERE user_id = ? FOR UPDATE`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	e.mock.ExpectRollback()
	if w := e.do(http.MethodPost, "/me/achievements", map[string]any{"title": "Another"}); w.Code != http.StatusNotFound {
		t.Fatalf("no profile: %d", w.Code)
	}
}

func TestCreateAchievementDefaultsAndCertOwnership(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`SELECT user_id FROM profiles WHERE user_id = ? FOR UPDATE`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(me))
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM achievements`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(49)) // default max 50
	e.mock.ExpectExec(q(`INSERT INTO achievements`)).WithArgs(me, nil, "Trophy", nil, "other", nil, nil).WillReturnResult(sqlmock.NewResult(6, 1))
	e.mock.ExpectCommit()
	e.expectAchievement(6, []any{int64(6), nil, "Trophy", nil, "other", nil, nil, achTime}, false)
	status(t, e.do(http.MethodPost, "/me/achievements", map[string]any{"title": " Trophy "}), http.StatusOK)

	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`SELECT user_id FROM profiles WHERE user_id = ? FOR UPDATE`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(me))
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM achievements`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	e.expectUpload(81, me, "", "") // someone else's upload
	e.mock.ExpectRollback()
	w := e.do(http.MethodPost, "/me/achievements", map[string]any{"title": "Trophy", "certificateUploadId": 81})
	if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["certificateUploadId"] == "" {
		t.Fatalf("cert: %d %s", w.Code, w.Body)
	}
}

func TestAchievementValidation(t *testing.T) {
	e := newEnv(t, nil, adult())
	for field, in := range map[string]map[string]any{
		"title":               {"level": "school"},
		"description":         {"title": "T", "description": strings.Repeat("d", 501)},
		"level":               {"title": "T", "level": "olympic"},
		"achievedOn":          {"title": "T", "achievedOn": "2026-06-16"}, // tomorrow in IST
		"sportId":             {"title": "T", "sportId": -1},
		"certificateUploadId": {"title": "T", "certificateUploadId": -2},
	} {
		w := e.do(http.MethodPost, "/me/achievements", in)
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields[field] == "" {
			t.Errorf("%s: %d %s", field, w.Code, w.Body)
		}
	}
	if w := e.do(http.MethodPost, "/me/achievements", map[string]any{"title": "T", "achievedOn": "1 June"}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad date: %d", w.Code)
	}
}

func TestUpdateAchievement(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.mock.ExpectBegin()
	e.expectAchievement(5, achRow(5, int64(80)), true)
	e.expectClaim(82, me, pmedia.PurposeAchievementCert)
	// Partial update: only level and certificate change; the rest is kept.
	e.mock.ExpectExec(q(`UPDATE achievements SET sport_id = ?, title = ?, description = ?, level = ?, achieved_on = ?, certificate_upload_id = ? WHERE id = ? AND user_id = ?`)).
		WithArgs(int64(1), "District champion", "Final", "state", "2025-12-01", int64(82), int64(5), me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.expectPurge(80, "achievement_certificate/7/old.pdf")
	row := achRow(5, int64(82))
	row[4] = "state"
	e.expectAchievement(5, row, false)
	e.expectViewURL(82, "achievement_certificate/7/new.pdf")
	var a Achievement
	_ = json.Unmarshal(status(t, e.do(http.MethodPut, "/me/achievements/5", map[string]any{"level": "state", "certificateUploadId": 82}), http.StatusOK).Data, &a)
	if a.Level != "state" || *a.CertificateUploadID != 82 {
		t.Fatalf("achievement = %+v", a)
	}

	// Another user's id → 404, nothing written.
	e.mock.ExpectBegin()
	e.expectAchievement(77, nil, true)
	e.mock.ExpectRollback()
	if w := e.do(http.MethodPut, "/me/achievements/77", map[string]any{"title": "Mine now"}); w.Code != http.StatusNotFound {
		t.Fatalf("other's: %d", w.Code)
	}
	// Validation after the merge: an empty title is refused.
	e.mock.ExpectBegin()
	e.expectAchievement(5, achRow(5, nil), true)
	e.mock.ExpectRollback()
	if w := e.do(http.MethodPut, "/me/achievements/5", map[string]any{"title": ""}); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty title: %d", w.Code)
	}
}

func TestDeleteAchievement(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.mock.ExpectBegin()
	e.expectAchievement(5, achRow(5, int64(80)), true)
	e.mock.ExpectExec(q(`DELETE FROM achievements WHERE id = ? AND user_id = ?`)).WithArgs(int64(5), me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.expectPurge(80, "achievement_certificate/7/c.pdf")
	status(t, e.do(http.MethodDelete, "/me/achievements/5", nil), http.StatusNoContent)

	e.mock.ExpectBegin()
	e.expectAchievement(77, nil, true)
	e.mock.ExpectRollback()
	if w := e.do(http.MethodDelete, "/me/achievements/77", nil); w.Code != http.StatusNotFound {
		t.Fatalf("other's: %d", w.Code)
	}
}

func TestAchievementsList(t *testing.T) {
	e := newEnv(t, nil, adult())
	e.expectPublicTarget("public", "private")
	e.mock.ExpectQuery(q(`SELECT id, sport_id, title, description, level, achieved_on, certificate_upload_id, created_at FROM achievements WHERE user_id = ? ORDER BY`)).WithArgs(other).
		WillReturnRows(achRows(achRow(5, int64(80)), achRow(4, nil)))
	e.expectViewURL(80, "achievement_certificate/9/c.pdf")
	var list []Achievement
	_ = json.Unmarshal(status(t, e.do(http.MethodGet, "/profiles/9/achievements", nil), http.StatusOK).Data, &list)
	if len(list) != 2 || list[0].CertificateURL == nil || list[1].CertificateURL != nil {
		t.Fatalf("list = %+v", list)
	}

	e2 := newEnv(t, nil, adult())
	e2.expectPublicTarget("followers", "public") // restricted: no list query
	b := status(t, e2.do(http.MethodGet, "/profiles/9/achievements", nil), http.StatusOK)
	if string(b.Data) != "[]" {
		t.Fatalf("restricted = %s", b.Data)
	}
}

// ---- user.deleted ----

func expectWipe(e *env, uploads ...int64) {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range uploads {
		rows.AddRow(id)
	}
	e.mock.ExpectQuery(q(`SELECT photo_upload_id FROM profiles WHERE user_id = ? AND photo_upload_id IS NOT NULL UNION ALL`)).WithArgs(me, me).WillReturnRows(rows)
	for _, id := range uploads {
		e.expectPurge(id, "k")
	}
	e.mock.ExpectBegin()
	e.mock.ExpectExec(q(`UPDATE profiles SET bio = NULL, city = NULL`)).WithArgs(me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`DELETE FROM profile_sports WHERE user_id = ?`)).WithArgs(me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`DELETE FROM user_stats WHERE user_id = ?`)).WithArgs(me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`DELETE FROM achievements WHERE user_id = ?`)).WithArgs(me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`DELETE FROM profile_views WHERE viewed_id = ? OR viewer_id = ?`)).WithArgs(me, me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
}

func TestUserDeletedIsIdempotent(t *testing.T) {
	e := newEnv(t, nil, adult())
	h := e.m.EventHandlers()[events.UserDeleted]
	ev := events.New(events.UserDeleted, me, events.UserPayload{UserID: me})
	expectWipe(e, 70, 80)
	expectWipe(e) // second delivery: nothing left to purge, the same statements are no-ops
	for i := 0; i < 2; i++ {
		if err := h(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := h(context.Background(), events.New(events.UserDeleted, 0, events.UserPayload{})); err != nil {
		t.Fatal("malformed payload is dropped")
	}
	e.mock.ExpectQuery(q(`SELECT photo_upload_id FROM profiles`)).WillReturnError(errors.New("db down"))
	if err := h(context.Background(), ev); err == nil {
		t.Fatal("db errors are returned for retry")
	}
}

// ---- counts.go ----

func TestFollowCountHelpers(t *testing.T) {
	e := newEnv(t, nil, adult())
	ctx := context.Background()
	e.mock.ExpectExec(q(`UPDATE profiles SET followers_count = followers_count + 1 WHERE user_id = ?`)).WithArgs(other).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`UPDATE profiles SET followers_count = followers_count - 1 WHERE user_id = ? AND followers_count > 0`)).WithArgs(other).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`UPDATE profiles SET following_count = following_count + 1 WHERE user_id = ?`)).WithArgs(me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`UPDATE profiles SET following_count = following_count - 1 WHERE user_id = ? AND following_count > 0`)).WithArgs(me).WillReturnResult(sqlmock.NewResult(0, 1))
	for _, err := range []error{
		AdjustFollowers(ctx, e.d.DB, other, 1), AdjustFollowers(ctx, e.d.DB, other, -1),
		AdjustFollowing(ctx, e.d.DB, me, 1), AdjustFollowing(ctx, e.d.DB, me, -1),
		AdjustFollowing(ctx, e.d.DB, me, 0), // no query
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	e.mock.ExpectQuery(q(`SELECT followers_count FROM profiles WHERE user_id = ?`)).WithArgs(other).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(4))
	e.mock.ExpectQuery(q(`SELECT followers_count FROM profiles WHERE user_id = ?`)).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"n"}))
	if n, err := FollowersCount(ctx, e.d.DB, other); err != nil || n != 4 {
		t.Fatalf("count = %d %v", n, err)
	}
	if n, err := FollowersCount(ctx, e.d.DB, me); err != nil || n != 0 {
		t.Fatalf("no profile = %d %v", n, err)
	}
}

func TestHideMinorCities(t *testing.T) {
	e := newEnv(t, nil, adult())
	city := func(s string) *string { return &s }
	cards := []userdir.Summary{{ID: me, City: city("Pune")}, {ID: other, City: city("Pune")}, {ID: 11, City: city("Nashik")}, {ID: 12}}
	e.mock.ExpectQuery(q(`SELECT u.id FROM users u WHERE u.is_minor = 1 AND u.id IN (?,?) AND NOT EXISTS (SELECT 1 FROM follows f WHERE f.follower_id = ? AND f.followee_id = u.id)`)).
		WithArgs(other, int64(11), me).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	if err := HideMinorCities(context.Background(), e.d, me, cards); err != nil {
		t.Fatal(err)
	}
	if cards[0].City == nil || cards[1].City == nil || cards[2].City != nil {
		t.Fatalf("cards = %+v", cards)
	}
	// Nothing to check → no query.
	if err := HideMinorCities(context.Background(), e.d, me, []userdir.Summary{{ID: me, City: city("Pune")}}); err != nil {
		t.Fatal(err)
	}
}

func TestListsVisible(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, nil, adult())
	if ok, err := ListsVisible(ctx, e.d, me, me); !ok || err != nil {
		t.Fatal("own lists are visible")
	}
	vis := func(v string) {
		rows := sqlmock.NewRows([]string{"visibility"})
		if v != "" {
			rows.AddRow(v)
		}
		e.mock.ExpectQuery(q(`SELECT visibility FROM profiles WHERE user_id = ?`)).WithArgs(other).WillReturnRows(rows)
	}
	for _, c := range []struct {
		vis, name string
		follows   *bool
		want      bool
	}{
		{vis: "public", want: true},
		{vis: "private", want: false},
		{vis: "", name: "no profile", want: false},
		{vis: "followers", follows: new(bool), want: false},
		{vis: "followers", name: "following", follows: func() *bool { b := true; return &b }(), want: true},
	} {
		vis(c.vis)
		if c.follows != nil {
			e.expectFollows(me, other, *c.follows)
		}
		if ok, err := ListsVisible(ctx, e.d, me, other); err != nil || ok != c.want {
			t.Errorf("%s %s: %v %v", c.vis, c.name, ok, err)
		}
	}
}
