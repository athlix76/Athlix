package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/auth/tokens"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
)

// bcryptCost is 12 (docs/security.md). Tests lower it to keep the suite fast.
var bcryptCost = 12

// totpIssuer is the label authenticator apps show.
const totpIssuer = "Athlix"

const (
	// Fallbacks for admin.login.max_per_ip / admin.login.window_minutes (seeded in 0010).
	defaultLoginMaxPerIP     = 20
	defaultLoginWindowMins   = 15
	defaultPasswordMinLength = 12 // admin.password_min_length
	defaultLoginEventsDays   = 365
	totpReplayTTL            = 90 * time.Second // covers the ±1 step skew window
	// bcrypt only reads the first 72 bytes, so longer passwords are refused (not a business rule).
	maxPasswordBytes = 72
	minPasswordFloor = 8
	maxUserAgentRune = 255 // admin_login_events.user_agent column size
	emailHashPurpose = "admin_email"
)

var roleNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)

// Service holds the admin rules.
type Service struct {
	d      *app.Deps
	repo   *repo
	tokens *tokens.Store
}

func newService(d *app.Deps) *Service {
	return &Service{d: d, repo: &repo{db: d.DB}, tokens: tokens.New(d.DB, d.Now)}
}

func (s *Service) now() time.Time { return s.d.Now().UTC() }

func errBadLogin() *httpx.Error {
	return httpx.Coded(401, httpx.CodeUnauthenticated, "Invalid email, password or code.")
}

var (
	dummyOnce sync.Once
	dummyHash []byte
)

// dummyCompare spends the same bcrypt time when the email is unknown (no account oracle).
func dummyCompare(pw string) {
	dummyOnce.Do(func() { dummyHash, _ = bcrypt.GenerateFromPassword([]byte("athlix-dummy-password"), bcryptCost) })
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(pw))
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// loginLimit is the per-IP login rate limit from config.
func (s *Service) loginLimit(ctx context.Context) (int, time.Duration) {
	n := s.d.Settings.Int(ctx, "admin.login.max_per_ip", defaultLoginMaxPerIP)
	mins := s.d.Settings.Int(ctx, "admin.login.window_minutes", defaultLoginWindowMins)
	if n < 1 {
		n = defaultLoginMaxPerIP
	}
	if mins < 1 {
		mins = defaultLoginWindowMins
	}
	return n, time.Duration(mins) * time.Minute
}

// truncateUA keeps at most maxUserAgentRune valid UTF-8 characters.
func truncateUA(ua string) string {
	ua = strings.ToValidUTF8(strings.TrimSpace(ua), "")
	if utf8.RuneCountInString(ua) <= maxUserAgentRune {
		return ua
	}
	return string([]rune(ua)[:maxUserAgentRune])
}

// Login checks password + TOTP, applies the lockout and starts an admin session. Every
// attempt that reaches the account lookup is written to admin_login_events in the same
// transaction (email as an HMAC; no password or code).
func (s *Service) Login(ctx context.Context, in loginBody, ci ClientInfo) (LoginResult, error) {
	limit, window := s.loginLimit(ctx)
	allowed, err := s.d.Limiter.Allow(ctx, "admin-login:"+ci.IP, limit, window)
	if err != nil {
		return LoginResult{}, err
	}
	if !allowed {
		return LoginResult{}, httpx.RateLimited("Too many login attempts. Try again later.")
	}
	email := normalizeEmail(in.Email)
	code := strings.TrimSpace(in.TotpCode)
	fields := map[string]string{}
	if email == "" {
		fields["email"] = "is required"
	}
	if in.Password == "" || len(in.Password) > 72 {
		fields["password"] = "is required"
	}
	if code == "" || len(code) > 10 {
		fields["totpCode"] = "is required"
	}
	if len(fields) > 0 {
		return LoginResult{}, httpx.Validation(fields)
	}

	st := s.d.Settings
	maxFailed := st.Int(ctx, "admin.max_failed_logins", 5)
	lockout := time.Duration(st.Int(ctx, "admin.lockout_minutes", 30)) * time.Minute
	refreshTTL := time.Duration(st.Int(ctx, "admin.refresh_token_ttl_hours", 12)) * time.Hour

	ev := loginEventRow{EmailHash: s.d.Crypto.Hash(emailHashPurpose, email), IP: ci.IP, UserAgent: truncateUA(ci.UserAgent), Outcome: OutcomeFailure}
	failed := false
	var adminID int64
	var refresh tokens.Issued
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		fail := func(reason string) error {
			failed, ev.Reason = true, reason
			return s.repo.insertLoginEvent(ctx, tx, ev)
		}
		a, err := s.repo.authByEmail(ctx, tx, email)
		if db.IsNoRows(err) {
			dummyCompare(in.Password)
			return fail(ReasonUnknownEmail)
		}
		if err != nil {
			return err
		}
		ev.AdminID = a.ID
		now := s.now()
		if a.Status != "active" {
			dummyCompare(in.Password)
			return fail(ReasonDisabled)
		}
		if a.LockedUntil != nil && a.LockedUntil.After(now) {
			dummyCompare(in.Password)
			return fail(ReasonLocked)
		}
		reason := ReasonBadPassword
		ok := bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(in.Password)) == nil
		if ok {
			reason, err = s.checkTOTP(ctx, a, code, now)
			if err != nil {
				return err
			}
			ok = reason == ReasonOK
		}
		if !ok {
			if err := s.countFailure(ctx, tx, a, now, maxFailed, lockout); err != nil {
				return err
			}
			return fail(reason)
		}
		if err := s.repo.recordLogin(ctx, tx, a.ID, now); err != nil {
			return err
		}
		ev.Outcome, ev.Reason = OutcomeSuccess, ReasonOK
		if err := s.repo.insertLoginEvent(ctx, tx, ev); err != nil {
			return err
		}
		adminID = a.ID
		refresh, err = s.tokens.Issue(ctx, tx, tokens.SubjectAdmin, a.ID, "", "", now.Add(refreshTTL))
		return err
	})
	if err != nil {
		return LoginResult{}, err
	}
	if failed {
		return LoginResult{}, errBadLogin()
	}
	me, err := s.repo.adminByID(ctx, s.d.DB, adminID, false)
	if err != nil {
		return LoginResult{}, err
	}
	pair, err := s.pair(ctx, me, refresh)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Tokens: pair, Admin: me}, nil
}

// countFailure adds one to failed_logins and locks the account for lockout when it
// reaches maxFailed (the counter then starts again).
func (s *Service) countFailure(ctx context.Context, tx *sql.Tx, a adminAuth, now time.Time, maxFailed int, lockout time.Duration) error {
	n := a.FailedLogins + 1
	var until *time.Time
	if n >= maxFailed {
		t := now.Add(lockout)
		until, n = &t, 0
	}
	return s.repo.recordFailure(ctx, tx, a.ID, n, until)
}

// checkTOTP validates the code and burns it so it cannot be replayed. It returns ReasonOK
// or the failure reason code.
func (s *Service) checkTOTP(ctx context.Context, a adminAuth, code string, now time.Time) (string, error) {
	if len(a.TotpEnc) == 0 {
		return ReasonNoTOTP, nil
	}
	secret, err := s.d.Crypto.DecryptString(a.TotpEnc)
	if err != nil {
		return "", err
	}
	ok, _ := totp.ValidateCustom(code, secret, now, totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if !ok {
		return ReasonBadTOTP, nil
	}
	fresh, err := s.d.Redis.SetNX(ctx, "admin:totp:"+strconv.FormatInt(a.ID, 10)+":"+code, "1", totpReplayTTL).Result()
	if err != nil {
		return "", err
	}
	if !fresh {
		return ReasonTOTPReplayed, nil
	}
	return ReasonOK, nil
}

func (s *Service) pair(ctx context.Context, a AdminUser, refresh tokens.Issued) (tokens.Pair, error) {
	ttl := time.Duration(s.d.Settings.Int(ctx, "admin.access_token_ttl_minutes", 15)) * time.Minute
	access, exp, err := s.d.Tokens.IssueAdmin(a.ID, a.Role.ID, a.Permissions, ttl)
	if err != nil {
		return tokens.Pair{}, err
	}
	return tokens.NewPair(access, exp, refresh), nil
}

// Refresh rotates the admin refresh token. The session keeps its original expiry (no
// sliding) and the access token gets the role's current permissions.
func (s *Service) Refresh(ctx context.Context, raw string) (tokens.Pair, error) {
	rot, err := s.tokens.Rotate(ctx, tokens.SubjectAdmin, raw,
		func(old time.Time) time.Time { return old },
		func(ctx context.Context, q db.Querier, id int64) error {
			status, err := s.repo.status(ctx, q, id)
			if db.IsNoRows(err) || (err == nil && status != "active") {
				return tokens.ErrInvalid
			}
			return err
		})
	if errors.Is(err, tokens.ErrInvalid) {
		return tokens.Pair{}, httpx.Unauthenticated()
	}
	if err != nil {
		return tokens.Pair{}, err
	}
	me, err := s.repo.adminByID(ctx, s.d.DB, rot.SubjectID, false)
	if err != nil {
		return tokens.Pair{}, err
	}
	return s.pair(ctx, me, rot.Token)
}

// Logout revokes the session family of raw, or every session of the admin.
func (s *Service) Logout(ctx context.Context, adminID int64, raw string) error {
	if raw == "" {
		return s.tokens.RevokeAll(ctx, nil, tokens.SubjectAdmin, adminID)
	}
	return s.tokens.RevokeFamilyOf(ctx, nil, tokens.SubjectAdmin, adminID, raw)
}

// Me is GET /admin/me.
func (s *Service) Me(ctx context.Context, adminID int64) (AdminUser, error) {
	a, err := s.repo.adminByID(ctx, s.d.DB, adminID, false)
	if db.IsNoRows(err) {
		return AdminUser{}, httpx.NotFound("Admin")
	}
	return a, err
}

// Dashboard is GET /admin/dashboard.
func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	return s.repo.dashboard(ctx, s.now())
}

// ---- admins ----

// newCredentials makes a random password and TOTP secret. Only the bcrypt hash and the
// encrypted secret are stored; the plain values are returned once.
func (s *Service) newCredentials(email string) (Credentials, string, []byte, error) {
	pw, err := crypto.RandomToken(18)
	if err != nil {
		return Credentials{}, "", nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	if err != nil {
		return Credentials{}, "", nil, err
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: email})
	if err != nil {
		return Credentials{}, "", nil, err
	}
	enc, err := s.d.Crypto.EncryptString(key.Secret())
	if err != nil {
		return Credentials{}, "", nil, err
	}
	return Credentials{TemporaryPassword: pw, TotpURI: key.URL()}, string(hash), enc, nil
}

func validateAdminFields(email, name string) (string, string, map[string]string) {
	fields := map[string]string{}
	email = normalizeEmail(email)
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 190 {
		fields["email"] = "must be a valid email address"
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		fields["name"] = "must be 1-80 characters"
	}
	return email, name, fields
}

// create inserts an admin inside tx. roleID 0 means look up roleName.
func (s *Service) create(ctx context.Context, tx *sql.Tx, email, name string, roleID int64, roleName string) (AdminUser, Credentials, error) {
	var role Role
	var err error
	if roleID > 0 {
		role, err = s.repo.role(ctx, tx, roleID, false)
	} else {
		role, err = s.repo.roleByName(ctx, tx, roleName)
	}
	if db.IsNoRows(err) {
		return AdminUser{}, Credentials{}, httpx.Validation(map[string]string{"roleId": "does not exist"})
	}
	if err != nil {
		return AdminUser{}, Credentials{}, err
	}
	creds, hash, enc, err := s.newCredentials(email)
	if err != nil {
		return AdminUser{}, Credentials{}, err
	}
	id, err := s.repo.insertAdmin(ctx, tx, email, name, hash, enc, s.d.Crypto.KeyVersion(), role.ID)
	if db.IsDuplicate(err) {
		return AdminUser{}, Credentials{}, httpx.Conflict("An admin with this email already exists.")
	}
	if err != nil {
		return AdminUser{}, Credentials{}, err
	}
	a := AdminUser{ID: id, Email: email, Name: name, Role: RoleRef{ID: role.ID, Name: role.Name}, Permissions: role.Permissions, Status: "active"}
	return a, creds, nil
}

// CreateAdmin creates an admin with role roleName (used by cmd/admincli). The returned
// credentials are shown once and never stored.
func CreateAdmin(ctx context.Context, d *app.Deps, email, name, roleName string) (AdminUser, Credentials, error) {
	s := newService(d)
	email, name, fields := validateAdminFields(email, name)
	if len(fields) > 0 {
		return AdminUser{}, Credentials{}, httpx.Validation(fields)
	}
	var a AdminUser
	var c Credentials
	err := db.WithTx(ctx, d.DB, func(tx *sql.Tx) error {
		var err error
		a, c, err = s.create(ctx, tx, email, name, 0, roleName)
		return err
	})
	return a, c, err
}

// ResetCredentialsByEmail replaces the password and TOTP secret of the admin with email
// and revokes their sessions (used by cmd/admincli).
func ResetCredentialsByEmail(ctx context.Context, d *app.Deps, email string) (Credentials, error) {
	s := newService(d)
	var c Credentials
	err := db.WithTx(ctx, d.DB, func(tx *sql.Tx) error {
		id, err := s.repo.idByEmail(ctx, tx, normalizeEmail(email))
		if db.IsNoRows(err) {
			return httpx.NotFound("Admin")
		}
		if err != nil {
			return err
		}
		c, err = s.reset(ctx, tx, id, normalizeEmail(email))
		return err
	})
	return c, err
}

func (s *Service) reset(ctx context.Context, tx *sql.Tx, id int64, email string) (Credentials, error) {
	creds, hash, enc, err := s.newCredentials(email)
	if err != nil {
		return Credentials{}, err
	}
	if err := s.repo.setCredentials(ctx, tx, id, hash, enc, s.d.Crypto.KeyVersion()); err != nil {
		return Credentials{}, err
	}
	return creds, s.tokens.RevokeAll(ctx, tx, tokens.SubjectAdmin, id)
}

// ListAdmins is GET /admin/admins.
func (s *Service) ListAdmins(ctx context.Context) ([]AdminUser, error) { return s.repo.listAdmins(ctx) }

// CreateViaAPI is POST /admin/admins; onTx writes the audit row in the same transaction.
func (s *Service) CreateViaAPI(ctx context.Context, in createAdminBody, onTx func(tx *sql.Tx, id int64) error) (CreatedAdmin, error) {
	email, name, fields := validateAdminFields(in.Email, in.Name)
	if in.RoleID <= 0 {
		fields["roleId"] = "is required"
	}
	if len(fields) > 0 {
		return CreatedAdmin{}, httpx.Validation(fields)
	}
	var out CreatedAdmin
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		a, c, err := s.create(ctx, tx, email, name, in.RoleID, "")
		if err != nil {
			return err
		}
		out = CreatedAdmin{Admin: a, Credentials: c}
		return onTx(tx, a.ID)
	})
	return out, err
}

// Update is PUT /admin/admins/:id. Disabling revokes the admin's sessions.
func (s *Service) Update(ctx context.Context, callerID, id int64, in updateAdminBody, onTx func(tx *sql.Tx, details map[string]any) error) (AdminUser, error) {
	fields := map[string]string{}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || utf8.RuneCountInString(n) > 80 {
			fields["name"] = "must be 1-80 characters"
		}
		in.Name = &n
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "disabled" {
		fields["status"] = "must be active or disabled"
	}
	if in.Status != nil && *in.Status == "disabled" && id == callerID {
		fields["status"] = "you cannot disable yourself"
	}
	if in.RoleID != nil && *in.RoleID <= 0 {
		fields["roleId"] = "does not exist"
	}
	if len(fields) > 0 {
		return AdminUser{}, httpx.Validation(fields)
	}
	var out AdminUser
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.adminByID(ctx, tx, id, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("Admin")
		}
		if err != nil {
			return err
		}
		next := cur
		details := map[string]any{}
		if in.Name != nil && *in.Name != cur.Name {
			next.Name = *in.Name
			details["nameChanged"] = true
		}
		if in.RoleID != nil && *in.RoleID != cur.Role.ID {
			role, err := s.repo.role(ctx, tx, *in.RoleID, false)
			if db.IsNoRows(err) {
				return httpx.Validation(map[string]string{"roleId": "does not exist"})
			}
			if err != nil {
				return err
			}
			next.Role, next.Permissions = RoleRef{ID: role.ID, Name: role.Name}, role.Permissions
			details["roleId"] = role.ID
		}
		if in.Status != nil && *in.Status != cur.Status {
			next.Status = *in.Status
			details["status"] = next.Status
		}
		if len(details) == 0 {
			out = cur
			return nil
		}
		losesSuper := cur.Status == "active" && slices.Contains(cur.Permissions, "*") &&
			(next.Status != "active" || !slices.Contains(next.Permissions, "*"))
		if losesSuper {
			n, err := s.repo.otherActiveSuperAdmins(ctx, tx, id)
			if err != nil {
				return err
			}
			if n == 0 {
				return httpx.InvalidState("At least one active super admin must remain.")
			}
		}
		if err := s.repo.updateAdmin(ctx, tx, id, next.Name, next.Role.ID, next.Status); err != nil {
			return err
		}
		if next.Status == "disabled" && cur.Status != "disabled" {
			if err := s.tokens.RevokeAll(ctx, tx, tokens.SubjectAdmin, id); err != nil {
				return err
			}
		}
		out = next
		return onTx(tx, details)
	})
	return out, err
}

// ResetCredentials is POST /admin/admins/:id/reset-credentials.
func (s *Service) ResetCredentials(ctx context.Context, id int64, onTx func(tx *sql.Tx) error) (Credentials, error) {
	var out Credentials
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		a, err := s.repo.adminByID(ctx, tx, id, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("Admin")
		}
		if err != nil {
			return err
		}
		if out, err = s.reset(ctx, tx, id, a.Email); err != nil {
			return err
		}
		return onTx(tx)
	})
	return out, err
}

// ---- roles ----

// ValidPermission reports whether p is in the closed permission list.
func ValidPermission(p string) bool { return slices.Contains(Permissions, p) }

func validatePerms(perms []string) ([]string, string) {
	if len(perms) == 0 {
		return nil, "must not be empty"
	}
	out := []string{}
	for _, p := range perms {
		if !ValidPermission(p) {
			return nil, "contains an unknown permission"
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out, ""
}

func validateDesc(d *string) (*string, string) {
	if d == nil {
		return nil, ""
	}
	v := strings.TrimSpace(*d)
	if utf8.RuneCountInString(v) > 255 {
		return nil, "must be at most 255 characters"
	}
	if v == "" {
		return nil, ""
	}
	return &v, ""
}

// Roles is GET /admin/roles.
func (s *Service) Roles(ctx context.Context) ([]Role, error) { return s.repo.roles(ctx) }

// CreateRole is POST /admin/roles.
func (s *Service) CreateRole(ctx context.Context, in createRoleBody, onTx func(tx *sql.Tx, id int64) error) (Role, error) {
	fields := map[string]string{}
	name := strings.TrimSpace(in.Name)
	if !roleNameRE.MatchString(name) {
		fields["name"] = "must be 2-48 lowercase letters, digits or underscores"
	}
	perms, msg := validatePerms(in.Permissions)
	if msg != "" {
		fields["permissions"] = msg
	}
	desc, msg := validateDesc(in.Description)
	if msg != "" {
		fields["description"] = msg
	}
	if len(fields) > 0 {
		return Role{}, httpx.Validation(fields)
	}
	var out Role
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		id, err := s.repo.insertRole(ctx, tx, name, desc, perms)
		if db.IsDuplicate(err) {
			return httpx.Conflict("A role with this name already exists.")
		}
		if err != nil {
			return err
		}
		out = Role{ID: id, Name: name, Description: desc, Permissions: perms}
		return onTx(tx, id)
	})
	return out, err
}

// UpdateRole is PUT /admin/roles/:id. super_admin cannot be changed. New permissions apply
// to each admin's next refresh.
func (s *Service) UpdateRole(ctx context.Context, id int64, in updateRoleBody, onTx func(tx *sql.Tx, details map[string]any) error) (Role, error) {
	fields := map[string]string{}
	var perms []string
	if in.Permissions != nil {
		var msg string
		if perms, msg = validatePerms(*in.Permissions); msg != "" {
			fields["permissions"] = msg
		}
	}
	desc, msg := validateDesc(in.Description)
	if msg != "" {
		fields["description"] = msg
	}
	if len(fields) > 0 {
		return Role{}, httpx.Validation(fields)
	}
	var out Role
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.role(ctx, tx, id, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("Role")
		}
		if err != nil {
			return err
		}
		if cur.Name == SuperAdminRole || slices.Contains(cur.Permissions, "*") {
			return httpx.InvalidState("The super admin role cannot be changed.")
		}
		next := cur
		details := map[string]any{"roleId": id}
		if in.Description != nil {
			next.Description = desc
			details["descriptionChanged"] = true
		}
		if perms != nil {
			next.Permissions = perms
			details["permissions"] = perms
		}
		if err := s.repo.updateRole(ctx, tx, id, next.Description, next.Permissions); err != nil {
			return err
		}
		out = next
		return onTx(tx, details)
	})
	return out, err
}

// ---- audit log ----

func (s *Service) parseAuditFilter(adminID, action, targetType, targetID string) (auditFilter, error) {
	f := auditFilter{Action: action, TargetType: targetType, TargetID: targetID}
	fields := map[string]string{}
	if adminID != "" {
		id, err := strconv.ParseInt(adminID, 10, 64)
		if err != nil || id <= 0 {
			fields["adminId"] = "must be a positive number"
		}
		f.AdminID = id
	}
	if len(action) > 64 {
		fields["action"] = "is too long"
	}
	if len(targetType) > 32 {
		fields["targetType"] = "is too long"
	}
	if len(targetID) > 64 {
		fields["targetId"] = "is too long"
	}
	if len(fields) > 0 {
		return f, httpx.Validation(fields)
	}
	return f, nil
}

// AuditLog is GET /admin/audit-log (newest first, keyset by id).
func (s *Service) AuditLog(ctx context.Context, f auditFilter) ([]AuditEntry, string, error) {
	rows, err := s.repo.auditLog(ctx, f)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	return rows, next, nil
}

// ---- change password ----

// passwordPolicy checks newPw against admin.password_min_length (clamped to 8..72) and,
// when email is set, the account's email name. It returns the field error, or "".
func (s *Service) passwordPolicy(ctx context.Context, email, current, newPw string) string {
	minLen := s.d.Settings.Int(ctx, "admin.password_min_length", defaultPasswordMinLength)
	if minLen < minPasswordFloor {
		minLen = minPasswordFloor
	}
	if minLen > maxPasswordBytes {
		minLen = maxPasswordBytes
	}
	if !utf8.ValidString(newPw) || utf8.RuneCountInString(newPw) < minLen || len(newPw) > maxPasswordBytes {
		return "must be " + strconv.Itoa(minLen) + " to " + strconv.Itoa(maxPasswordBytes) + " bytes long"
	}
	if strings.TrimSpace(newPw) != newPw {
		return "must not start or end with a space"
	}
	if newPw == current {
		return "must differ from the current password"
	}
	if local, _, ok := strings.Cut(normalizeEmail(email), "@"); ok && len(local) >= 3 && strings.Contains(strings.ToLower(newPw), local) {
		return "must not contain your email name"
	}
	return ""
}

// ChangePassword is POST /admin/me/password. The caller proves the current password and a
// fresh TOTP code. A wrong password or code counts towards the lockout like a failed login
// and returns 422 (not 401, so the client keeps its session). On success every refresh
// token of the admin is revoked and a new session is issued; onTx writes the audit row.
func (s *Service) ChangePassword(ctx context.Context, adminID int64, in changePasswordBody, onTx func(tx *sql.Tx) error) (ChangePasswordResult, error) {
	code := strings.TrimSpace(in.TotpCode)
	fields := map[string]string{}
	if in.CurrentPassword == "" || len(in.CurrentPassword) > maxPasswordBytes {
		fields["currentPassword"] = "is required"
	}
	if in.NewPassword == "" {
		fields["newPassword"] = "is required"
	}
	if code == "" || len(code) > 10 {
		fields["totpCode"] = "is required"
	}
	if len(fields) == 0 {
		// Checks that need no account run first, so a weak password does not burn the code.
		if msg := s.passwordPolicy(ctx, "", in.CurrentPassword, in.NewPassword); msg != "" {
			fields["newPassword"] = msg
		}
	}
	if len(fields) > 0 {
		return ChangePasswordResult{}, httpx.Validation(fields)
	}
	st := s.d.Settings
	maxFailed := st.Int(ctx, "admin.max_failed_logins", 5)
	lockout := time.Duration(st.Int(ctx, "admin.lockout_minutes", 30)) * time.Minute
	refreshTTL := time.Duration(st.Int(ctx, "admin.refresh_token_ttl_hours", 12)) * time.Hour

	var refresh tokens.Issued
	var verr error
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		a, err := s.repo.authByID(ctx, tx, adminID)
		if db.IsNoRows(err) {
			return httpx.NotFound("Admin")
		}
		if err != nil {
			return err
		}
		now := s.now()
		if a.Status != "active" || (a.LockedUntil != nil && a.LockedUntil.After(now)) {
			return httpx.Unauthenticated()
		}
		ok := bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(in.CurrentPassword)) == nil
		if ok {
			reason, err := s.checkTOTP(ctx, a, code, now)
			if err != nil {
				return err
			}
			ok = reason == ReasonOK
		}
		if !ok {
			// Commit the failure count; the 422 is returned after the transaction.
			verr = httpx.Validation(map[string]string{"currentPassword": "password or code is incorrect"})
			return s.countFailure(ctx, tx, a, now, maxFailed, lockout)
		}
		if msg := s.passwordPolicy(ctx, a.Email, in.CurrentPassword, in.NewPassword); msg != "" {
			verr = httpx.Validation(map[string]string{"newPassword": msg})
			return nil
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcryptCost)
		if err != nil {
			return err
		}
		if err := s.repo.setPassword(ctx, tx, a.ID, string(hash)); err != nil {
			return err
		}
		if err := s.tokens.RevokeAll(ctx, tx, tokens.SubjectAdmin, a.ID); err != nil {
			return err
		}
		if refresh, err = s.tokens.Issue(ctx, tx, tokens.SubjectAdmin, a.ID, "", "", now.Add(refreshTTL)); err != nil {
			return err
		}
		return onTx(tx)
	})
	if err != nil {
		return ChangePasswordResult{}, err
	}
	if verr != nil {
		return ChangePasswordResult{}, verr
	}
	me, err := s.repo.adminByID(ctx, s.d.DB, adminID, false)
	if err != nil {
		return ChangePasswordResult{}, err
	}
	pair, err := s.pair(ctx, me, refresh)
	if err != nil {
		return ChangePasswordResult{}, err
	}
	return ChangePasswordResult{Tokens: pair}, nil
}

// ---- login events ----

func (s *Service) parseLoginEventFilter(adminID, email, outcome string) (loginEventFilter, error) {
	f := loginEventFilter{}
	fields := map[string]string{}
	if adminID != "" {
		id, err := strconv.ParseInt(adminID, 10, 64)
		if err != nil || id <= 0 {
			fields["adminId"] = "must be a positive number"
		}
		f.AdminID = id
	}
	if e := normalizeEmail(email); e != "" {
		if len(e) > 190 {
			fields["email"] = "is too long"
		}
		f.EmailHash = s.d.Crypto.Hash(emailHashPurpose, e)
	}
	switch outcome {
	case "", OutcomeSuccess, OutcomeFailure:
		f.Outcome = outcome
	default:
		fields["outcome"] = "must be success or failure"
	}
	if len(fields) > 0 {
		return f, httpx.Validation(fields)
	}
	return f, nil
}

// LoginEvents is GET /admin/login-events (newest first, keyset by id).
func (s *Service) LoginEvents(ctx context.Context, f loginEventFilter) ([]LoginEvent, string, error) {
	rows, err := s.repo.loginEvents(ctx, f)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	return rows, next, nil
}

// purgeLoginEvents is the admin.purge_login_events job: it deletes login events older than
// admin.login_events_retention_days, 500 rows at a time, at most 20 batches per run.
func (s *Service) purgeLoginEvents(ctx context.Context) error {
	days := s.d.Settings.Int(ctx, "admin.login_events_retention_days", defaultLoginEventsDays)
	if days < 1 {
		days = defaultLoginEventsDays
	}
	cutoff := s.now().Add(-time.Duration(days) * 24 * time.Hour)
	const batch, maxBatches = 500, 20
	for i := 0; i < maxBatches; i++ {
		n, err := s.repo.purgeLoginEvents(ctx, cutoff, batch)
		if err != nil {
			return err
		}
		if n < batch {
			return nil
		}
	}
	return nil
}
