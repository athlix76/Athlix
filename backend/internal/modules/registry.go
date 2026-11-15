// Package modules lists every feature module. Add a new module here (one line) and nowhere else.
package modules

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/admin"
	"athlix/backend/internal/modules/ads"
	"athlix/backend/internal/modules/appconfig"
	"athlix/backend/internal/modules/auth"
	"athlix/backend/internal/modules/chat"
	"athlix/backend/internal/modules/community"
	"athlix/backend/internal/modules/consultation"
	"athlix/backend/internal/modules/event"
	"athlix/backend/internal/modules/feed"
	"athlix/backend/internal/modules/follow"
	"athlix/backend/internal/modules/guardian"
	"athlix/backend/internal/modules/listingpayment"
	"athlix/backend/internal/modules/locationidentity"
	"athlix/backend/internal/modules/media"
	"athlix/backend/internal/modules/moderation"
	"athlix/backend/internal/modules/notification"
	"athlix/backend/internal/modules/organiser"
	"athlix/backend/internal/modules/profile"
	"athlix/backend/internal/modules/registration"
	"athlix/backend/internal/modules/search"
	"athlix/backend/internal/modules/sport"
	"athlix/backend/internal/modules/subscription"
	"athlix/backend/internal/modules/user"
	"athlix/backend/internal/modules/verification"
)

// All builds every module in registration order.
func All(d *app.Deps) []app.Module {
	return []app.Module{
		auth.New(d),
		user.New(d),
		guardian.New(d),
		verification.New(d),
		locationidentity.New(d),
		sport.New(d),
		profile.New(d),
		follow.New(d),
		appconfig.New(d),
		admin.New(d),
		media.New(d),
		organiser.New(d),
		event.New(d),
		registration.New(d),
		listingpayment.New(d),
		subscription.New(d),
		moderation.New(d),
		search.New(d),
		ads.New(d),
		consultation.New(d),
		chat.New(d),
		community.New(d),
		notification.New(d),
		feed.New(d),
	}
}
