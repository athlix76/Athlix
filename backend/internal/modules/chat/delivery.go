package chat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi/types"
	"github.com/redis/go-redis/v9"

	"athlix/backend/internal/platform/logger"
)

// Delivery posts one JSON frame to one WebSocket connection.
type Delivery interface {
	Send(ctx context.Context, connID string, frame []byte) error
}

// ErrGone means the connection no longer exists (API Gateway 410); callers drop it.
var ErrGone = errors.New("chat: connection gone")

// connTTL is the lifetime of ws:conn / ws:user keys, refreshed on ping.
const connTTL = 2 * time.Hour

func connKey(connID string) string { return "ws:conn:" + connID }
func userKey(userID int64) string  { return "ws:user:" + strconv.FormatInt(userID, 10) }

// ---- API Gateway Management API (AWS) ----

type apiGatewayDelivery struct {
	c *apigatewaymanagementapi.Client
}

// NewAPIGatewayDelivery posts frames through the API Gateway Management API at endpoint
// (WS_MANAGEMENT_URL, https://<api-id>.execute-api.<region>.amazonaws.com/<stage>).
func NewAPIGatewayDelivery(cfg aws.Config, endpoint string) Delivery {
	c := apigatewaymanagementapi.NewFromConfig(cfg, func(o *apigatewaymanagementapi.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	return &apiGatewayDelivery{c: c}
}

func (a *apiGatewayDelivery) Send(ctx context.Context, connID string, frame []byte) error {
	_, err := a.c.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{ConnectionId: aws.String(connID), Data: frame})
	var gone *apigwtypes.GoneException
	if errors.As(err, &gone) {
		return ErrGone
	}
	return err
}

// ---- in-process hub (ENV=local) ----

// LocalHub delivers frames to connections served by this process (the local /v1/chat/ws route).
type LocalHub struct {
	mu    sync.RWMutex
	conns map[string]func(ctx context.Context, frame []byte) error
}

// NewLocalHub builds an empty hub.
func NewLocalHub() *LocalHub {
	return &LocalHub{conns: map[string]func(context.Context, []byte) error{}}
}

// Register attaches a writer for connID.
func (h *LocalHub) Register(connID string, write func(ctx context.Context, frame []byte) error) {
	h.mu.Lock()
	h.conns[connID] = write
	h.mu.Unlock()
}

// Unregister detaches connID.
func (h *LocalHub) Unregister(connID string) {
	h.mu.Lock()
	delete(h.conns, connID)
	h.mu.Unlock()
}

// Send implements Delivery. Unknown connections are ErrGone.
func (h *LocalHub) Send(ctx context.Context, connID string, frame []byte) error {
	h.mu.RLock()
	w, ok := h.conns[connID]
	h.mu.RUnlock()
	if !ok {
		return ErrGone
	}
	return w(ctx, frame)
}

// ---- connection registry (Redis) ----

// RegisterConnection stores ws:conn:<connId> → userId and adds it to ws:user:<userId>.
func (s *Service) RegisterConnection(ctx context.Context, connID string, userID int64) error {
	pipe := s.d.Redis.TxPipeline()
	pipe.Set(ctx, connKey(connID), strconv.FormatInt(userID, 10), connTTL)
	pipe.SAdd(ctx, userKey(userID), connID)
	pipe.Expire(ctx, userKey(userID), connTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// RemoveConnection deletes the connection from both keys (idempotent).
func (s *Service) RemoveConnection(ctx context.Context, connID string) error {
	uid, err := s.ConnectionUser(ctx, connID)
	if errors.Is(err, ErrGone) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.dropConn(ctx, uid, connID)
}

func (s *Service) dropConn(ctx context.Context, userID int64, connID string) error {
	pipe := s.d.Redis.TxPipeline()
	pipe.Del(ctx, connKey(connID))
	pipe.SRem(ctx, userKey(userID), connID)
	_, err := pipe.Exec(ctx)
	return err
}

// ConnectionUser returns the user of a live connection, or ErrGone.
func (s *Service) ConnectionUser(ctx context.Context, connID string) (int64, error) {
	v, err := s.d.Redis.Get(ctx, connKey(connID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, ErrGone
	}
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, ErrGone
	}
	return id, nil
}

// Touch refreshes the TTL of the connection keys (ping).
func (s *Service) Touch(ctx context.Context, connID string) error {
	uid, err := s.ConnectionUser(ctx, connID)
	if err != nil {
		return err
	}
	pipe := s.d.Redis.TxPipeline()
	pipe.Expire(ctx, connKey(connID), connTTL)
	pipe.Expire(ctx, userKey(uid), connTTL)
	_, err = pipe.Exec(ctx)
	return err
}

// SendToConnection posts one frame to one connection and drops it when gone.
func (s *Service) SendToConnection(ctx context.Context, connID string, frame []byte) error {
	err := s.delivery.Send(ctx, connID, frame)
	if errors.Is(err, ErrGone) {
		if rerr := s.RemoveConnection(ctx, connID); rerr != nil {
			logger.From(ctx).Warn("chat drop connection", slog.String("err", rerr.Error()))
		}
	}
	return err
}

// ---- fan-out ----

// frame is one server → client WebSocket frame.
type frame struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

func encodeFrame(typ string, data any) []byte {
	b, _ := json.Marshal(frame{Type: typ, Data: data})
	return b
}

// fanOut posts frame to every live connection of users, except skipConn. Gone connections
// are removed from Redis. Delivery errors never fail the caller (the app polls on resume).
func (s *Service) fanOut(ctx context.Context, users []int64, skipConn string, f []byte) {
	if len(users) == 0 {
		return
	}
	pipe := s.d.Redis.Pipeline()
	cmds := make([]*redis.StringSliceCmd, len(users))
	for i, u := range users {
		cmds[i] = pipe.SMembers(ctx, userKey(u))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		logger.From(ctx).Error("chat fan-out lookup", slog.String("err", err.Error()))
		return
	}
	type target struct {
		user int64
		conn string
	}
	var targets []target
	for i, c := range cmds {
		for _, conn := range c.Val() {
			if conn != skipConn {
				targets = append(targets, target{users[i], conn})
			}
		}
	}
	const workers = 8
	ch := make(chan target)
	var wg sync.WaitGroup
	for w := 0; w < workers && w < len(targets); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range ch {
				err := s.delivery.Send(ctx, t.conn, f)
				switch {
				case errors.Is(err, ErrGone):
					if derr := s.dropConn(ctx, t.user, t.conn); derr != nil {
						logger.From(ctx).Warn("chat drop connection", slog.Int64("userId", t.user), slog.String("err", derr.Error()))
					}
				case err != nil:
					logger.From(ctx).Warn("chat delivery failed", slog.Int64("userId", t.user), slog.String("err", err.Error()))
				}
			}
		}()
	}
	for _, t := range targets {
		ch <- t
	}
	close(ch)
	wg.Wait()
}

// fanOutToMembers posts frame to every active member of convID (optionally except one user).
func (s *Service) fanOutToMembers(ctx context.Context, convID, exceptUser int64, skipConn string, f []byte) {
	ids, err := activeMemberIDs(ctx, s.d.DB, convID)
	if err != nil {
		logger.From(ctx).Error("chat fan-out members", slog.Int64("conversationId", convID), slog.String("err", err.Error()))
		return
	}
	if exceptUser > 0 {
		out := ids[:0]
		for _, id := range ids {
			if id != exceptUser {
				out = append(out, id)
			}
		}
		ids = out
	}
	s.fanOut(ctx, ids, skipConn, f)
}
