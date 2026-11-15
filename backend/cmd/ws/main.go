// Command ws is the Lambda behind the API Gateway WebSocket API. It owns only the transport:
// $connect redeems a single-use ticket, $disconnect drops the connection, and every other
// route key is handed to chat.Service.HandleFrame. Replies go back through the API Gateway
// Management API. Locally the api serves the same protocol at /v1/chat/ws.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/chat"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

func main() {
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") == "" {
		fmt.Println("cmd/ws only runs in AWS Lambda behind an API Gateway WebSocket API.")
		fmt.Println("For local development run cmd/api (ENV=local) and connect to ws://localhost:8080/v1/chat/ws?ticket=<ticket from POST /v1/chat/ws-ticket>.")
		return
	}
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	d, cleanup, err := app.Bootstrap(ctx, cfg)
	if err != nil {
		slog.Error("bootstrap", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer cleanup()

	h := &handler{d: d, s: chat.NewService(d), explicit: cfg.WSManagementURL != "" && d.AWS != nil}
	if !h.explicit {
		d.Log.Warn("WS_MANAGEMENT_URL not set; deriving the management endpoint from the first request")
	}
	lambda.Start(h.handle)
}

type handler struct {
	d        *app.Deps
	s        *chat.Service
	explicit bool // delivery configured from WS_MANAGEMENT_URL
	once     sync.Once
}

// handle serves one WebSocket event. Tickets, tokens and message bodies are never logged.
func (h *handler) handle(ctx context.Context, req events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	rc := req.RequestContext
	ctx = logger.With(ctx, h.d.Log.With(slog.String("route", rc.RouteKey), slog.String("requestId", rc.RequestID)))
	h.ensureDelivery(ctx, rc)
	log := logger.From(ctx)

	switch rc.RouteKey {
	case "$connect":
		uid, err := h.s.ConsumeTicket(ctx, req.QueryStringParameters["ticket"])
		if errors.Is(err, chat.ErrInvalidTicket) {
			return status(http.StatusUnauthorized), nil
		}
		if err != nil {
			log.Error("ws connect", slog.String("err", err.Error()))
			return status(http.StatusInternalServerError), nil
		}
		if err := h.s.RegisterConnection(ctx, rc.ConnectionID, uid); err != nil {
			log.Error("ws register", slog.Int64("userId", uid), slog.String("err", err.Error()))
			return status(http.StatusInternalServerError), nil
		}
		log.Info("ws connected", slog.Int64("userId", uid))
		return status(http.StatusOK), nil

	case "$disconnect":
		if err := h.s.RemoveConnection(ctx, rc.ConnectionID); err != nil {
			log.Warn("ws disconnect", slog.String("err", err.Error()))
		}
		return status(http.StatusOK), nil
	}

	// $default and the named actions (sendMessage, typing, markRead, ping).
	uid, err := h.s.ConnectionUser(ctx, rc.ConnectionID)
	if errors.Is(err, chat.ErrGone) {
		_ = h.s.SendToConnection(ctx, rc.ConnectionID, unauthenticatedFrame())
		return status(http.StatusOK), nil
	}
	if err != nil {
		log.Error("ws lookup", slog.String("err", err.Error()))
		return status(http.StatusInternalServerError), nil
	}
	for _, f := range h.s.HandleFrame(ctx, uid, rc.ConnectionID, []byte(req.Body)) {
		if err := h.s.SendToConnection(ctx, rc.ConnectionID, f); err != nil {
			if !errors.Is(err, chat.ErrGone) {
				log.Warn("ws reply", slog.Int64("userId", uid), slog.String("err", err.Error()))
			}
			break
		}
	}
	return status(http.StatusOK), nil
}

// ensureDelivery points the service at https://<domain>/<stage> when WS_MANAGEMENT_URL is
// unset (default execute-api domain only; set WS_MANAGEMENT_URL for a custom domain).
func (h *handler) ensureDelivery(ctx context.Context, rc events.APIGatewayWebsocketProxyRequestContext) {
	if h.explicit || h.d.AWS == nil || rc.DomainName == "" {
		return
	}
	h.once.Do(func() {
		h.s.SetDelivery(chat.NewAPIGatewayDelivery(*h.d.AWS, "https://"+rc.DomainName+"/"+rc.Stage))
		logger.From(ctx).Info("ws delivery endpoint derived from request")
	})
}

func status(code int) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{StatusCode: code}
}

// unauthenticatedFrame tells a client whose connection is unknown to reconnect with a ticket.
func unauthenticatedFrame() []byte {
	e := httpx.Unauthenticated()
	b, _ := json.Marshal(map[string]any{"type": "error", "data": map[string]any{"clientMsgId": nil, "code": e.Code, "message": e.Message}})
	return b
}
