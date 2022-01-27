package rcsnooper

import "golang.org/x/oauth2"

func (c *Controller) GetDriveClientID() string {
	return c.drive.clientID
}

func (c *Controller) GetDriveClientSecret() string {
	return c.drive.clientSecret
}

func (c *Controller) GetDriveToken() *oauth2.Token {
	return c.drive.token
}

func (c *Controller) GetDriveInfos() (clientID, clientSecret string, token *oauth2.Token) {
	return c.drive.clientID, c.drive.clientSecret, c.drive.token
}
