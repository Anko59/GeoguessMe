package chat

import (
	"context"
	"log/slog"

	"geoguessme/internal/models"
)

// SerializePrivacyChange orders preference commits against socket writes in
// this process. Pending events are always reauthorized after the barrier; no
// cached block set or disconnect command is used as an authorization boundary.
func (h *Hub) SerializePrivacyChange(change func() error) error {
	h.privacy.Lock()
	defer h.privacy.Unlock()
	return change()
}

// WithPrivacyRead protects an external delivery (such as Web Push) using the
// same preference commit barrier as WebSocket delivery.
func (h *Hub) WithPrivacyRead(deliver func()) {
	h.privacy.RLock()
	defer h.privacy.RUnlock()
	deliver()
}

func (c *Client) deliver(message models.Message) error {
	return c.deliverAuthorized(message, func(current models.Message) error { return c.conn.WriteJSON(current) })
}

// Only transient protocol errors lack an ID. Persisted system announcements
// (for example Party Time) contain user content and require reauthorization too.
func (c *Client) deliverAuthorized(message models.Message, write func(models.Message) error) error {
	c.hub.privacy.RLock()
	defer c.hub.privacy.RUnlock()
	if c.hub.Delivery != nil && message.ID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), c.hub.persistTimeout)
		current, err := c.hub.Delivery(ctx, message, c.userID)
		cancel()
		if err != nil {
			slog.Warn("chat delivery authorization failed", "error", err)
			return nil
		}
		if current == nil {
			return nil
		}
		message = *current
	}
	return write(message)
}
