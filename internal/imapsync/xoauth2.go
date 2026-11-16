package imapsync

import (
	"errors"

	"github.com/emersion/go-sasl"
)

// xoauth2Client implements the XOAUTH2 SASL mechanism used by Gmail and
// Outlook: https://developers.google.com/gmail/imap/xoauth2-protocol
type xoauth2Client struct {
	user, token string
}

var _ sasl.Client = (*xoauth2Client)(nil)

func newXOAuth2Client(user, token string) sasl.Client {
	return &xoauth2Client{user: user, token: token}
}

func (c *xoauth2Client) Start() (mech string, ir []byte, err error) {
	return "XOAUTH2", []byte("user=" + c.user + "\x01auth=Bearer " + c.token + "\x01\x01"), nil
}

// On failure the server sends a JSON error as a challenge; the client must
// answer with an empty response, after which the server returns NO.
func (c *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	if len(challenge) > 0 {
		return []byte{}, nil
	}
	return nil, errors.New("xoauth2: unexpected server challenge")
}
