# features/communities

User-created groups (docs/api.md "Communities"). A community **is** a chat conversation of type
`community`, so `Community.id` is also its conversation id. Its chat opens with
`Routes.conversation(id)`.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `CommunitiesScreen` | `/communities` | Discover (search, sport and state filters) and Mine tabs; create button |
| `CommunityDetailScreen` | `/communities/:id` | info; join / request / accept invite / open chat / leave, chosen by `myStatus`; edit, manage and report |
| `CommunityFormScreen` | `/communities/new`, `/communities/:id/edit` | create or edit (edit sends only the changed fields); avatar uploaded as `community_avatar`; the owner can delete |
| `CommunityManageScreen` | `/communities/:id/manage` | tabs: Members (role changes by the owner only; remove and ban), Requests (approve / reject), Invite (people search, then invite; shows who was skipped). The owner can "Make owner" an admin (then becomes an admin) |

## Endpoints

- `POST /communities`, `GET /communities?q&sportId&state&cursor`, `GET /me/communities`
- `GET|PUT|DELETE /communities/:id`, `POST /communities/:id/join`
- `GET /communities/:id/join-requests`, `POST /communities/:id/join-requests/:userId/approve|reject`
- `POST /communities/:id/invites`, `POST /communities/:id/invites/accept`
- `POST /communities/:id/members/:userId/remove|ban`, `PUT /communities/:id/members/:userId/role`
- `POST /communities/:id/transfer-ownership` `{userId}`
- Through chat: `GET /chat/conversations/:id/members` and `POST /chat/conversations/:id/leave`
- `GET /search/people?q` for the invite picker, plus the `POST /media/uploads` flow for the avatar

## Rules

- Limits (`chat.max_communities_created_per_user`, `chat.group_max_members`) are shown from
  public config and enforced by the server. The minor rules are server-side only.
- The server checks every permission. The UI hides actions the caller's role cannot do.
