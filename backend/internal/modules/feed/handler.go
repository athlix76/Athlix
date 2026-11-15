package feed

import (
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

// handler adapts HTTP to the service. The caller always comes from the verified token.
type handler struct{ svc *Service }

// page parses ?cursor=&limit=; limit may only lower feed.page_size.
func (h *handler) page(c *gin.Context) (httpx.PageParams, error) {
	def := h.svc.PageSize(c.Request.Context())
	p, err := httpx.ParsePage(c, def)
	if err != nil {
		return p, err
	}
	if p.Limit > def {
		p.Limit = def
	}
	return p, nil
}

func (h *handler) feed(c *gin.Context) {
	mode := c.DefaultQuery("mode", ModeFollowing)
	if mode != ModeFollowing && mode != ModeDiscover {
		httpx.Fail(c, httpx.Validation(map[string]string{"mode": "must be following or discover"}))
		return
	}
	p, err := h.page(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	posts, next, err := h.svc.Feed(c.Request.Context(), authn.UserID(c), mode, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, posts, httpx.Meta{NextCursor: next})
}

func (h *handler) userPosts(c *gin.Context) {
	userID, err := httpx.PathID(c, "userId")
	if err != nil {
		httpx.Fail(c, errUserNotFound)
		return
	}
	p, err := h.page(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	posts, next, err := h.svc.UserPosts(c.Request.Context(), authn.UserID(c), userID, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, posts, httpx.Meta{NextCursor: next})
}

func (h *handler) createPost(c *gin.Context) {
	var in CreatePostInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	post, err := h.svc.CreatePost(c.Request.Context(), authn.User(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, post)
}

func (h *handler) getPost(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errPostNotFound)
		return
	}
	post, err := h.svc.GetPost(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, post)
}

func (h *handler) deletePost(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errPostNotFound)
		return
	}
	if err := h.svc.DeletePost(c.Request.Context(), authn.UserID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) like(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errPostNotFound)
		return
	}
	res, err := h.svc.Like(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *handler) unlike(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errPostNotFound)
		return
	}
	res, err := h.svc.Unlike(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *handler) comments(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errPostNotFound)
		return
	}
	p, err := h.page(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.Comments(c.Request.Context(), authn.UserID(c), id, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) addComment(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errPostNotFound)
		return
	}
	var in CommentInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	cm, err := h.svc.AddComment(c.Request.Context(), authn.UserID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, cm)
}

func (h *handler) deleteComment(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errCommentNotFound)
		return
	}
	if err := h.svc.DeleteComment(c.Request.Context(), authn.UserID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}
