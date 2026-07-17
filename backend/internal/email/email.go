package email

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTP TLS modes. See Config.SMTPTLS and deployment documentation.
const (
	ModeOff      = "off"
	ModeStartTLS = "starttls"
	ModeTLS      = "tls"
)

// Sender abstracts outbound mail so tests and local gameplay can swap a no-op.
type Sender interface {
	Send(to, subject, body string) error
}

// SMTP sends mail over a real SMTP connection with an explicit TLS policy.
type SMTP struct {
	Host        string
	Port        int
	Username    string
	Password    string
	From        string
	TLSMode     string
	DialTimeout time.Duration
	Timeout     time.Duration
	// TLSConfig overrides the default verified TLS configuration. Leave nil in
	// production; tests inject a trusted root certificate pool.
	TLSConfig *tls.Config
}

// Send delivers a single plain-text message. It refuses to transmit
// credentials over a plaintext link and validates the configured TLS mode.
func (s SMTP) Send(to, subject, body string) error {
	mode := normalizeMode(s.TLSMode)
	switch mode {
	case ModeOff, ModeStartTLS, ModeTLS:
	default:
		return fmt.Errorf("email: unsupported SMTP_TLS mode %q", s.TLSMode)
	}
	if strings.TrimSpace(s.Host) == "" {
		// Mail delivery is optional in development/test stacks (Mailpit).
		return nil
	}
	if s.Username != "" && mode == ModeOff {
		return errors.New("email: authenticated SMTP requires TLS (starttls or tls)")
	}
	if err := validateAddress(s.From); err != nil {
		return fmt.Errorf("email: sender: %w", err)
	}
	if err := validateAddress(to); err != nil {
		return fmt.Errorf("email: recipient: %w", err)
	}

	client, err := s.connect()
	if err != nil {
		return err
	}
	defer client.Close()

	if s.Username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("email: server does not advertise AUTH")
		}
		if err := client.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
	}
	if err := client.Mail(s.From); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("email: RCPT TO: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := writer.Write(s.message(to, subject, body)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("email: close body: %w", err)
	}
	return client.Quit()
}

func (s SMTP) connect() (*smtp.Client, error) {
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	dialer := &net.Dialer{Timeout: s.dialTimeoutOrDefault()}
	tlsConfig := s.tlsConfig()

	switch normalizeMode(s.TLSMode) {
	case ModeTLS:
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("email: implicit TLS dial: %w", err)
		}
		return s.smtpFrom(deadlineConn{Conn: conn, timeout: s.sendTimeoutOrDefault()})
	case ModeStartTLS:
		conn, err := dialer.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("email: dial: %w", err)
		}
		client, err := s.smtpFrom(deadlineConn{Conn: conn, timeout: s.sendTimeoutOrDefault()})
		if err != nil {
			return nil, err
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("email: STARTTLS: %w", err)
		}
		return client, nil
	default: // ModeOff
		conn, err := dialer.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("email: dial: %w", err)
		}
		return s.smtpFrom(deadlineConn{Conn: conn, timeout: s.sendTimeoutOrDefault()})
	}
}

func (s SMTP) tlsConfig() *tls.Config {
	if s.TLSConfig != nil {
		clone := s.TLSConfig.Clone()
		if clone.ServerName == "" {
			clone.ServerName = s.Host
		}
		if clone.MinVersion == 0 {
			clone.MinVersion = tls.VersionTLS12
		}
		return clone
	}
	return &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
}

func (s SMTP) smtpFrom(conn net.Conn) (*smtp.Client, error) {
	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func (s SMTP) dialTimeoutOrDefault() time.Duration {
	if s.DialTimeout > 0 {
		return s.DialTimeout
	}
	return 10 * time.Second
}

func (s SMTP) sendTimeoutOrDefault() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return 30 * time.Second
}

// deadlineConn resets the connection deadline on every read and write so the
// handshake, auth, and DATA transfer are each bounded by the configured send
// timeout even though net/smtp exposes no per-operation deadline API.
type deadlineConn struct {
	net.Conn
	timeout time.Duration
}

func (c deadlineConn) Read(b []byte) (int, error) {
	_ = c.Conn.SetReadDeadline(time.Now().Add(c.timeout))
	return c.Conn.Read(b)
}

func (c deadlineConn) Write(b []byte) (int, error) {
	_ = c.Conn.SetWriteDeadline(time.Now().Add(c.timeout))
	return c.Conn.Write(b)
}

// message assembles a valid MIME message with UTF-8 capable headers and a
// base64-encoded utf-8 body so non-ASCII content survives every hop.
func (s SMTP) message(to, subject, body string) []byte {
	var builder strings.Builder
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("From: " + s.From + "\r\n")
	builder.WriteString("To: " + to + "\r\n")
	builder.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	builder.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	builder.WriteString("Message-ID: " + messageID(s.From) + "\r\n")
	builder.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: base64\r\n")
	builder.WriteString("\r\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	for offset := 0; offset < len(encoded); offset += 76 {
		end := offset + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		builder.WriteString(encoded[offset:end])
		builder.WriteString("\r\n")
	}
	return []byte(builder.String())
}

func messageID(from string) string {
	domain := "localhost"
	if parts := strings.Split(from, "@"); len(parts) == 2 && parts[1] != "" {
		domain = parts[1]
	}
	return fmt.Sprintf("<%d.geoguessme@%s>", time.Now().UnixNano(), domain)
}

func validateAddress(address string) error {
	parsed, err := mail.ParseAddress(address)
	if err != nil {
		return err
	}
	if !strings.Contains(parsed.Address, "@") {
		return errors.New("invalid email address")
	}
	return nil
}

func normalizeMode(mode string) string {
	if mode == "" {
		return ModeOff
	}
	return strings.ToLower(strings.TrimSpace(mode))
}

// Noop discards mail. Used when SMTP is intentionally disabled.
type Noop struct{}

func (Noop) Send(_, _, _ string) error { return nil }
