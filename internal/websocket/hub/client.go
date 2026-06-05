package hub

import (
	"sync"

	"github.com/gofiber/contrib/websocket"
)

const sendBufferSize = 256

type Client struct {
	TenantID string
	UserID   string
	Conn     *websocket.Conn
	send     chan []byte
	once     sync.Once
}

func NewClient(tenantID, userID string, conn *websocket.Conn) *Client {
	return &Client{
		TenantID: tenantID,
		UserID:   userID,
		Conn:     conn,
		send:     make(chan []byte, sendBufferSize),
	}
}

func (c *Client) Close() {
	c.once.Do(func() {
		close(c.send)
		if c.Conn != nil {
			_ = c.Conn.Close()
		}
	})
}

func (c *Client) ReadPump(h *Hub, onMessage func(*Client, []byte)) {
	defer h.Unregister(c)

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			return
		}
		if onMessage != nil {
			onMessage(c, message)
		}
	}
}

func (c *Client) WritePump() {
	for message := range c.send {
		if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

// Send enqueues a payload to be written to the WebSocket connection.
// It is safe for concurrent use by event publishers.
func (c *Client) Send(payload []byte) {
	select {
	case c.send <- payload:
	default:
		// Drop messages when the client is slow; avoids blocking hub.
	}
}
