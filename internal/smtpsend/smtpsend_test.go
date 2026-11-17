package smtpsend

import (
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

type received struct {
	mu   sync.Mutex
	from string
	to   []string
	data string
}

type backend struct{ got *received }

func (b backend) NewSession(*smtp.Conn) (smtp.Session, error) { return &session{got: b.got}, nil }

type session struct {
	got    *received
	authed bool
}

func (s *session) AuthMechanisms() []string { return []string{sasl.Plain} }
func (s *session) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, user, pass string) error {
		if user == "me@example.com" && pass == "right" {
			s.authed = true
			return nil
		}
		return errors.New("bad credentials")
	}), nil
}
func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	if !s.authed {
		return &smtp.SMTPError{Code: 530, Message: "auth required"}
	}
	s.got.mu.Lock()
	s.got.from = from
	s.got.mu.Unlock()
	return nil
}
func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.got.mu.Lock()
	s.got.to = append(s.got.to, to)
	s.got.mu.Unlock()
	return nil
}
func (s *session) Data(r io.Reader) error {
	b, _ := io.ReadAll(r)
	s.got.mu.Lock()
	s.got.data = string(b)
	s.got.mu.Unlock()
	return nil
}
func (s *session) Reset()        {}
func (s *session) Logout() error { return nil }

func start(t *testing.T) (Config, *received) {
	t.Helper()
	got := &received{}
	srv := smtp.NewServer(backend{got: got})
	srv.AllowInsecureAuth = true
	srv.Domain = "localhost"
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	insecure = true
	t.Cleanup(func() { insecure = false })
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(port)
	return Config{Host: host, Port: p, Username: "me@example.com", Password: "right"}, got
}

func TestSend(t *testing.T) {
	cfg, got := start(t)
	raw := []byte("From: me@example.com\r\nTo: a@example.com\r\nSubject: hi\r\n\r\nHello\r\n")
	if err := Send(cfg, "me@example.com", []string{"a@example.com", "hidden@example.com"}, raw); err != nil {
		t.Fatalf("Send: %v", err)
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	if got.from != "me@example.com" || len(got.to) != 2 || got.to[1] != "hidden@example.com" {
		t.Errorf("envelope from=%q to=%v", got.from, got.to)
	}
	if !strings.Contains(got.data, "Subject: hi") {
		t.Errorf("data = %q", got.data)
	}
}

func TestSendWrongPassword(t *testing.T) {
	cfg, _ := start(t)
	cfg.Password = "wrong"
	err := Send(cfg, "me@example.com", []string{"a@example.com"}, []byte("Subject: x\r\n\r\nx\r\n"))
	if err == nil {
		t.Fatal("sent with the wrong password")
	}
}

func TestSendNeedsServer(t *testing.T) {
	if err := Send(Config{}, "me@example.com", []string{"a@example.com"}, nil); err == nil {
		t.Fatal("expected an error without an SMTP host")
	}
}
