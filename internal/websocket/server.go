package websocket

import (
	"encoding/json"

	ws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/websocket/hub"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

// Register registers the websocket endpoint (GET /ws).
// Clients authenticate with a JWT access token via query param: ?token=...
func Register(app *fiber.App, wsHub *hub.Hub, jwt *jwtmanager.Manager, log *logger.Logger) {
	if app == nil || wsHub == nil || jwt == nil {
		return
	}

	app.Get("/ws", ws.New(func(conn *ws.Conn) {
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

		_ = log
	}))
}
