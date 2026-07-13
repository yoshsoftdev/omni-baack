package websocket

import (
	"context"
	"encoding/json"
	"time"

	ws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

// Register registers websocket endpoints (/ws and /api/v1/ws).
func Register(
	app *fiber.App,
	wsHub *hub.Hub,
	jwt *jwtmanager.Manager,
	log *logger.Logger,
	presence *inbox.PresenceService,
	collisions *inbox.CollisionService,
) {
	if app == nil || wsHub == nil || jwt == nil {
		return
	}
	handler := newWSHandler(wsHub, jwt, log, presence, collisions)
	app.Get("/ws", ws.New(handler))
	app.Get("/api/v1/ws", ws.New(handler))
}

func newWSHandler(
	wsHub *hub.Hub,
	jwt *jwtmanager.Manager,
	log *logger.Logger,
	presence *inbox.PresenceService,
	collisions *inbox.CollisionService,
) func(*ws.Conn) {
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

		// ReadPump unregisters before returning. Delay clear so brief reconnects
		// (token refresh / network blip) do not leave a stuck empty view.
		if collisions != nil {
			teamID := claims.TeamID
			userID := claims.UserID
			go func() {
				time.Sleep(3 * time.Second)
				if wsHub.HasUserConnection(teamID.String(), userID.String()) {
					return
				}
				_ = collisions.ClearUserPresence(context.Background(), teamID, userID)
			}()
		}
		if presence != nil && !wsHub.HasUserConnection(claims.TeamID.String(), claims.UserID.String()) {
			_ = presence.SetOffline(context.Background(), claims.TeamID, claims.UserID)
		}

		_ = log
	}
}
