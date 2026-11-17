// Package imapsync keeps the local store in step with IMAP servers.
package imapsync

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/HamzaBaghdadi/pigeon/internal/xoauth2"
)

// Security is how the connection is encrypted. Plaintext is deliberately not
// offered: credentials and mail never cross the network unencrypted.
type Security string

const (
	SecurityTLS      Security = "tls"      // implicit TLS (port 993)
	SecurityStartTLS Security = "starttls" // upgrade on port 143
)

// Config is how to reach and log in to one IMAP account.
type Config struct {
	Host     string
	Port     int
	Security Security
	Username string
	// Exactly one of Password or AccessToken (OAuth, sent with XOAUTH2) is set.
	Password    string
	AccessToken string
}

// dialInsecure is only set by tests (in-memory server without TLS).
var dialInsecure = false

func (c Config) addr() string { return net.JoinHostPort(c.Host, fmt.Sprint(c.Port)) }

// Connect dials and logs in.
func Connect(cfg Config) (*imapclient.Client, error) {
	return connect(cfg, nil)
}

// connect dials and logs in, delivering server pushes to h (may be nil).
func connect(cfg Config, h *imapclient.UnilateralDataHandler) (*imapclient.Client, error) {
	opts := &imapclient.Options{
		TLSConfig:             &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12},
		Dialer:                &net.Dialer{Timeout: 20 * time.Second},
		UnilateralDataHandler: h,
	}
	var (
		c   *imapclient.Client
		err error
	)
	switch {
	case dialInsecure:
		c, err = imapclient.DialInsecure(cfg.addr(), opts)
	case cfg.Security == SecurityStartTLS:
		c, err = imapclient.DialStartTLS(cfg.addr(), opts)
	case cfg.Security == SecurityTLS || cfg.Security == "":
		c, err = imapclient.DialTLS(cfg.addr(), opts)
	default:
		return nil, fmt.Errorf("unsupported security %q", cfg.Security)
	}
	if err != nil {
		return nil, friendlyDialError(cfg, err)
	}
	if cfg.AccessToken != "" {
		err = c.Authenticate(xoauth2.NewClient(cfg.Username, cfg.AccessToken))
	} else {
		err = c.Login(cfg.Username, cfg.Password).Wait()
	}
	if err != nil {
		c.Close()
		return nil, friendlyLoginError(err)
	}
	return c, nil
}

// Verify checks that the settings work: connect, log in, list folders.
func Verify(cfg Config) error {
	c, err := Connect(cfg)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err := c.List("", "%", nil).Collect(); err != nil {
		return fmt.Errorf("logged in, but couldn't list folders: %w", err)
	}
	_ = c.Logout().Wait()
	return nil
}

func friendlyDialError(cfg Config, err error) error {
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.As(err, &dnsErr):
		return fmt.Errorf("couldn't find the server %s. Check the server name", cfg.Host)
	case errors.As(err, &certErr):
		return fmt.Errorf("the server's security certificate isn't valid for %s, so Pigeon didn't connect", cfg.Host)
	case strings.Contains(err.Error(), "timeout"), strings.Contains(err.Error(), "refused"):
		return fmt.Errorf("couldn't reach %s on port %d. Check the server and port", cfg.Host, cfg.Port)
	}
	return fmt.Errorf("couldn't connect to %s: %w", cfg.Host, err)
}

func friendlyLoginError(err error) error {
	msg := err.Error()
	if strings.Contains(strings.ToUpper(msg), "AUTHENTICATIONFAILED") || strings.Contains(strings.ToLower(msg), "invalid credentials") ||
		strings.Contains(strings.ToLower(msg), "authentication failed") || strings.Contains(strings.ToLower(msg), "login failed") {
		return ErrAuth
	}
	return fmt.Errorf("sign-in failed: %w", err)
}

// ErrAuth means the server rejected the username or password.
var ErrAuth = errors.New("the server rejected the email or password")
