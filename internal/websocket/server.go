package websocket

import (
	"context"
	"encoding/json"

	ws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

// Register registers websocket endpoints (/ws and /api/v1/ws).
func Register(app *fiber.App, wsHub *hub.Hub, jwt *jwtmanager.Manager, log *logger.Logger, presence *inbox.PresenceService) {
	if app == nil || wsHub == nil || jwt == nil {
		return
	}
	handler := newWSHandler(wsHub, jwt, log, presence)
	app.Get("/ws", ws.New(handler))
	app.Get("/api/v1/ws", ws.New(handler))
}

func newWSHandler(wsHub *hub.Hub, jwt *jwtmanager.Manager, log *logger.Logger, presence *inbox.PresenceService) func(*ws.Conn) {
	return func(conn *ws.Conn) {
		token := conn.Query("token")
		if token == "" {
			_ = conn.WriteJSON(fiber.Map{"type": "error", "message": "missing token"})
			_ = conn.Close()
			return
		}

		claims, err := jwt.ParseAccess(token)
		if err != nil {
			_ = conn.WriteJSON(fiber.Map{"type": "error", "message": "invalid token"})
			_ = conn.Close()
			return
		}

		client := hub.NewClient(claims.TeamID.String(), claims.UserID.String(), conn)
		wsHub.Register(client)
		if presence != nil {
			_ = presence.SetOnline(context.Background(), claims.TeamID, claims.UserID)
		}

		welcome := fiber.Map{
			"type":    "ws.welcome",
			"team_id": claims.TeamID.String(),
			"user_id": claims.UserID.String(),
			"role":    claims.Role,
		}
		if payload, err := json.Marshal(welcome); err == nil {
			client.Send(payload)
		}

		go client.WritePump()
		client.ReadPump(wsHub, nil)
		if presence != nil {
			_ = presence.SetOffline(context.Background(), claims.TeamID, claims.UserID)
		}

		_ = log
		_ = uuid.Nil
	}
}
