// Package smtpsend delivers outgoing messages over SMTP submission.
package smtpsend

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/HamzaBaghdadi/pigeon/internal/xoauth2"
)

// Config is how to reach and sign in to an SMTP server. Plaintext connections
// are not offered.
type Config struct {
	Host     string
	Port     int
	Security string // "tls" (implicit, port 465) or "starttls" (port 587)
	Username string
	// Exactly one of Password or AccessToken (OAuth, sent with XOAUTH2) is set.
	Password    string
	AccessToken string
}

// insecure is only set by tests (local server without TLS).
var insecure = false

// ErrAuth means the server rejected the credentials.
var ErrAuth = errors.New("the mail server rejected the sign-in for sending")

// Send delivers raw to the recipients, with from as the envelope sender.
func Send(cfg Config, from string, to []string, raw []byte) error {
	if cfg.Host == "" {
		return errors.New("this account has no outgoing (SMTP) server. Add one in the account's server settings")
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, 20*time.Second)
	if err != nil {
		return fmt.Errorf("couldn't reach the outgoing server %s: %w", addr, err)
	}
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}

	var c *smtp.Client
	switch {
	case insecure:
		c = smtp.NewClient(conn)
	case cfg.Security == "starttls":
		c, err = smtp.NewClientStartTLS(conn, tlsCfg)
	case cfg.Security == "tls" || cfg.Security == "":
		c = smtp.NewClient(tls.Client(conn, tlsCfg))
	default:
		conn.Close()
		return fmt.Errorf("unsupported security %q", cfg.Security)
	}
	if err != nil {
		conn.Close()
		return fmt.Errorf("couldn't start a secure connection to %s: %w", cfg.Host, err)
	}
	defer c.Close()
	c.CommandTimeout = time.Minute
	c.SubmissionTimeout = 5 * time.Minute

	var auth sasl.Client
	if cfg.AccessToken != "" {
		auth = xoauth2.NewClient(cfg.Username, cfg.AccessToken)
	} else {
		auth = sasl.NewPlainClient("", cfg.Username, cfg.Password)
	}
	if err := c.Auth(auth); err != nil {
		var se *smtp.SMTPError
		if errors.As(err, &se) && (se.Code == 535 || se.Code == 534) {
			return ErrAuth
		}
		return fmt.Errorf("couldn't sign in to the outgoing server: %w", err)
	}
	if err := c.SendMail(from, to, bytes.NewReader(raw)); err != nil {
		var se *smtp.SMTPError
		if errors.As(err, &se) {
			return fmt.Errorf("the server refused the message: %s", se.Message)
		}
		return fmt.Errorf("sending failed: %w", err)
	}
	return c.Quit()
}
