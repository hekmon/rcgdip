package plex

import (
	"fmt"

	uuid "github.com/nu7hatch/gouuid"
)

const (
	stateClientIDKey = "clientID"
)

func (c *Controller) getClientID() (clientID string, err error) {
	// First try to recover a current clientID
	exists, err := c.state.Get(stateClientIDKey, &clientID)
	if err != nil {
		err = fmt.Errorf("failed to recover clientID from local state: %w", err)
		return
	}
	if exists {
		c.logger.Debugf("[Plex] clientID recovered from state: %s", clientID)
		return
	}
	// If no clientID found, generate a new one
	clientIDRaw, err := uuid.NewV4()
	if err != nil {
		err = fmt.Errorf("failed to generate a new UUID: %w", err)
		return
	}
	clientID = clientIDRaw.String()
	c.logger.Debugf("[Plex] new clientID generated: %s", clientID)
	// And save it
	if err = c.state.Set(stateClientIDKey, clientID); err != nil {
		err = fmt.Errorf("failed to save the newly generated UUID: %w", err)
	}
	return
}
