package community

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"time"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/userdir"
)

const lockTTL = 10 * time.Second

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// JoinRequests lists pending requests (owner/admin), oldest first.
func (s *Service) JoinRequests(ctx context.Context, caller, id int64, p httpx.PageParams) ([]JoinRequest, string, error) {
	var rows []requestRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.lockFor(ctx, tx, id, caller, true, false); err != nil {
			return err
		}
		var after time.Time
		if p.Cursor.Time != "" {
			t, err := time.Parse(time.RFC3339Nano, p.Cursor.Time)
			if err != nil {
				return httpx.Validation(map[string]string{"cursor": "is invalid"})
			}
			after = t
		}
		var err error
		rows, err = s.r.joinRequests(ctx, tx, id, after, p.Cursor.ID, p.Limit+1)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		last := rows[len(rows)-1]
		next = httpx.EncodeCursor(httpx.Cursor{ID: last.UserID, Time: last.RequestedAt.UTC().Format(time.RFC3339Nano)})
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	sums, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	out := make([]JoinRequest, 0, len(rows))
	for _, r := range rows {
		if sm, ok := sums[r.UserID]; ok {
			out = append(out, JoinRequest{User: sm, RequestedAt: r.RequestedAt.UTC()})
		}
	}
	return out, next, nil
}

// Approve activates a pending request and emits community.join_approved after commit.
func (s *Service) Approve(ctx context.Context, caller, id, userID int64) error {
	var owner int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		l, err := s.lockFor(ctx, tx, id, caller, true, false)
		if err != nil {
			return err
		}
		owner = l.OwnerID
		st, _, found, err := s.r.member(ctx, tx, id, userID)
		if err != nil {
			return err
		}
		if !found || st != MemberPending {
			return httpx.NotFound("join request")
		}
		if l.MemberCount >= s.d.Settings.Int(ctx, "chat.group_max_members", 500) {
			return errFull
		}
		if err := s.r.setMemberStatus(ctx, tx, id, userID, MemberActive); err != nil {
			return err
		}
		return s.r.adjustCount(ctx, tx, id, +1)
	})
	if err != nil {
		return err
	}
	s.publish(ctx, events.CommunityJoinApproved, caller, events.CommunityJoinPayload{ConversationID: id, UserID: userID, OwnerID: owner})
	return nil
}

// Reject deletes a pending request (owner/admin). The user may ask again later.
func (s *Service) Reject(ctx context.Context, caller, id, userID int64) error {
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.lockFor(ctx, tx, id, caller, true, false); err != nil {
			return err
		}
		st, _, found, err := s.r.member(ctx, tx, id, userID)
		if err != nil {
			return err
		}
		if !found || st != MemberPending {
			return httpx.NotFound("join request")
		}
		return s.r.deletePending(ctx, tx, id, userID)
	})
}

// Invite invites users (owner/admin). Users who cannot be invited are listed in skipped:
// self, unknown / inactive, already a member / pending / banned, blocked either way with the
// inviter or the owner, minors failing minor.dm_policy against the inviter, and minors when
// the community is public and minor.can_join_public_communities is off.
func (s *Service) Invite(ctx context.Context, inviter userdir.Basic, id int64, in InviteInput) (InviteResult, error) {
	if len(in.UserIDs) == 0 {
		return InviteResult{}, httpx.Validation(map[string]string{"userIds": "is required"})
	}
	if len(in.UserIDs) > maxInviteBatch {
		return InviteResult{}, httpx.Validation(map[string]string{"userIds": "must have at most 50 ids"})
	}
	ids := uniq(in.UserIDs)
	res := InviteResult{Invited: []int64{}, Skipped: []int64{}}
	var fresh []int64 // users moved into 'invited' by this call (they get community.invited)
	var owner int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		res = InviteResult{Invited: []int64{}, Skipped: []int64{}}
		fresh = fresh[:0]
		l, err := s.lockFor(ctx, tx, id, inviter.ID, true, false)
		if err != nil {
			return err
		}
		owner = l.OwnerID
		statuses, err := s.r.memberStatuses(ctx, tx, id, ids)
		if err != nil {
			return err
		}
		for _, uid := range ids {
			ok, err := s.canInvite(ctx, inviter, l, uid, statuses[uid])
			if err != nil {
				return err
			}
			if !ok {
				res.Skipped = append(res.Skipped, uid)
				continue
			}
			if statuses[uid] != MemberInvited {
				if err := s.r.upsertMember(ctx, tx, id, uid, MemberInvited); err != nil {
					return err
				}
				fresh = append(fresh, uid)
			}
			res.Invited = append(res.Invited, uid)
		}
		return nil
	})
	if err != nil {
		return InviteResult{}, err
	}
	logger.From(ctx).Info("community invites", slog.Int64("communityId", id), slog.Int64("userId", inviter.ID),
		slog.Int("invited", len(res.Invited)), slog.Int("skipped", len(res.Skipped)))
	// After commit: one community.invited per newly invited user. Re-inviting someone who is
	// already invited emits nothing, so a repeated call does not notify twice.
	for _, uid := range fresh {
		s.emit(ctx, EventInvited, inviter.ID, id, InvitedPayload{
			CommunityJoinPayload: events.CommunityJoinPayload{ConversationID: id, UserID: uid, OwnerID: owner},
			InviterID:            inviter.ID,
		})
	}
	return res, nil
}

func (s *Service) canInvite(ctx context.Context, inviter userdir.Basic, l lockRow, uid int64, status string) (bool, error) {
	if uid <= 0 || uid == inviter.ID {
		return false, nil
	}
	switch status {
	case MemberActive, MemberPending, MemberBanned:
		return false, nil
	}
	target, err := s.d.Users.Get(ctx, uid)
	if db.IsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if target.Status != "active" || target.OnboardingStep != "done" {
		return false, nil
	}
	blocked, err := s.d.Users.BlockedEitherWay(ctx, inviter.ID, uid)
	if err != nil || blocked {
		return false, err
	}
	if l.OwnerID != 0 && l.OwnerID != inviter.ID {
		blocked, err := s.d.Users.BlockedEitherWay(ctx, l.OwnerID, uid)
		if err != nil || blocked {
			return false, err
		}
	}
	if target.IsMinor || inviter.IsMinor {
		if s.minorBlocked(ctx, target.IsMinor, l.Visibility) {
			return false, nil
		}
		switch s.d.Settings.String(ctx, "minor.dm_policy", "mutual_follow") {
		case "anyone":
		case "mutual_follow":
			mutual, err := s.d.Users.MutualFollow(ctx, inviter.ID, uid)
			if err != nil || !mutual {
				return false, err
			}
		default: // "none" or unknown → refuse (fail safe)
			return false, nil
		}
	}
	return true, nil
}

// target reads and checks a member a manager acts on. Admins can't act on the owner or
// other admins; nobody acts on themselves here (leaving is POST /chat/conversations/:id/leave).
func (s *Service) target(ctx context.Context, tx *sql.Tx, l lockRow, id, caller, userID int64) (status string, found bool, err error) {
	if userID == caller {
		return "", false, httpx.InvalidState("Use leave to exit a community.")
	}
	st, role, found, err := s.r.member(ctx, tx, id, userID)
	if err != nil {
		return "", false, err
	}
	if found && (role == RoleOwner || (role == RoleAdmin && l.MyRole != RoleOwner)) {
		return "", false, httpx.Forbidden("You can't do this to the owner or another admin.")
	}
	return st, found, nil
}

// RemoveMember removes an active member, or cancels an invite / pending request.
func (s *Service) RemoveMember(ctx context.Context, caller, id, userID int64) error {
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		l, err := s.lockFor(ctx, tx, id, caller, true, false)
		if err != nil {
			return err
		}
		st, found, err := s.target(ctx, tx, l, id, caller, userID)
		if err != nil {
			return err
		}
		if !found || (st != MemberActive && st != MemberInvited && st != MemberPending) {
			return httpx.NotFound("member")
		}
		if err := s.r.setMemberStatus(ctx, tx, id, userID, MemberRemoved); err != nil {
			return err
		}
		if st == MemberActive {
			return s.r.adjustCount(ctx, tx, id, -1)
		}
		return nil
	})
}

// BanMember bans a user: removes them if active and stops them joining again. A user with no
// membership row can be banned too.
func (s *Service) BanMember(ctx context.Context, caller, id, userID int64) error {
	if userID <= 0 {
		return httpx.NotFound("member")
	}
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		l, err := s.lockFor(ctx, tx, id, caller, true, false)
		if err != nil {
			return err
		}
		st, _, err := s.target(ctx, tx, l, id, caller, userID)
		if err != nil {
			return err
		}
		if st == MemberBanned {
			return nil
		}
		if err := s.r.ban(ctx, tx, id, userID); err != nil {
			return err
		}
		if st == MemberActive {
			return s.r.adjustCount(ctx, tx, id, -1)
		}
		return nil
	})
}

// SetRole makes an active member an admin or a plain member (owner only).
func (s *Service) SetRole(ctx context.Context, caller, id, userID int64, in RoleInput) error {
	if in.Role != RoleAdmin && in.Role != RoleMember {
		return httpx.Validation(map[string]string{"role": "must be admin or member"})
	}
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		l, err := s.lockFor(ctx, tx, id, caller, true, true)
		if err != nil {
			return err
		}
		st, found, err := s.target(ctx, tx, l, id, caller, userID)
		if err != nil {
			return err
		}
		if !found || st != MemberActive {
			return httpx.NotFound("member")
		}
		return s.r.setRole(ctx, tx, id, userID, in.Role)
	})
}

// TransferOwnership hands the community to an active admin (owner only). In one tx the new
// owner gets role 'owner', the caller becomes 'admin', conversations.owner_id moves and a
// community_ownership_transfers row is written; community.ownership_transferred is published
// after commit. The new owner must still be under their own community limit.
func (s *Service) TransferOwnership(ctx context.Context, caller, id int64, in TransferInput) error {
	if in.UserID <= 0 {
		return httpx.Validation(map[string]string{"userId": "is required"})
	}
	if in.UserID == caller {
		return httpx.InvalidState("You already own this community.")
	}
	target, err := s.d.Users.Get(ctx, in.UserID)
	if db.IsNoRows(err) {
		return httpx.NotFound("member")
	}
	if err != nil {
		return err
	}
	if target.Status != "active" {
		return httpx.NotFound("member")
	}
	limit, err := s.ownLimit(ctx, in.UserID)
	if err != nil {
		return err
	}
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.lockFor(ctx, tx, id, caller, true, true); err != nil {
			return err
		}
		st, role, found, err := s.r.member(ctx, tx, id, in.UserID)
		if err != nil {
			return err
		}
		if !found || st != MemberActive {
			return httpx.NotFound("member")
		}
		if role != RoleAdmin {
			return httpx.InvalidState("Make this member an admin before transferring ownership.")
		}
		n, err := s.r.countOwned(ctx, tx, in.UserID)
		if err != nil {
			return err
		}
		if n >= limit {
			return httpx.LimitReached("This member has reached the number of communities they can own.")
		}
		if err := s.r.transferOwner(ctx, tx, id, caller, in.UserID); err != nil {
			return err
		}
		return s.r.logTransfer(ctx, tx, id, caller, in.UserID)
	})
	if err != nil {
		return err
	}
	logger.From(ctx).Info("community ownership transferred", slog.Int64("communityId", id),
		slog.Int64("fromUserId", caller), slog.Int64("toUserId", in.UserID))
	s.emit(ctx, EventOwnershipTransferred, caller, id, OwnershipPayload{
		CommunityJoinPayload: events.CommunityJoinPayload{ConversationID: id, UserID: in.UserID, OwnerID: in.UserID},
		PreviousOwnerID:      caller,
	})
	return nil
}

func uniq(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
