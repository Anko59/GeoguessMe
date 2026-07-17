package email

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// startTestSMTPServer runs a minimal ESMTP server on 127.0.0.1. When
// startTLS is true it advertises STARTTLS and upgrades; when implicitTLS is
// true the listener itself is wrapped in TLS. It records delivered message
// bodies for assertions.
func startTestSMTPServer(t *testing.T, startTLS, implicitTLS, advertiseAuth bool) (string, tls.Certificate) {
	t.Helper()
	cert, key := selfSignedCert(t, "127.0.0.1")
	_ = key
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12, ClientAuth: tls.NoClientCert}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if implicitTLS {
		listener = tls.NewListener(listener, tlsConfig)
	}
	t.Cleanup(func() { _ = listener.Close() })

	var mu sync.Mutex
	delivered := make([]string, 0)
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if len(delivered) == 0 {
			t.Errorf("expected at least one delivered message")
		}
	})

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleSMTP(conn, tlsConfig, startTLS, advertiseAuth, func(body string) {
				mu.Lock()
				delivered = append(delivered, body)
				mu.Unlock()
			})
		}
	}()

	_, port, _ := net.SplitHostPort(listener.Addr().String())
	return port, cert
}

func handleSMTP(conn net.Conn, tlsConfig *tls.Config, startTLS, advertiseAuth bool, deliver func(string)) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	write("220 test ESMTP")
	inTLS := false
	inData := false
	var body strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if inData {
			if trimmed == "." {
				inData = false
				deliver(body.String())
				body.Reset()
				write("250 OK")
				continue
			}
			body.WriteString(line)
			continue
		}
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			var caps []string
			if startTLS {
				caps = append(caps, "250-STARTTLS")
			}
			if advertiseAuth {
				caps = append(caps, "250-AUTH PLAIN")
			}
			caps = append(caps, "250 HELP")
			write(strings.Join(caps, "\r\n"))
		case upper == "STARTTLS":
			write("220 ready")
			tlsConn := tls.Server(conn, tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			inTLS = true
			_ = inTLS
			conn = tlsConn
			reader = bufio.NewReader(conn)
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			write("235 OK")
		case strings.HasPrefix(upper, "MAIL FROM"):
			write("250 OK")
		case strings.HasPrefix(upper, "RCPT TO"):
			write("250 OK")
		case upper == "DATA":
			write("354 start")
			inData = true
		case upper == "NOOP":
			write("250 OK")
		case upper == "RSET":
			write("250 OK")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("500 unrecognized")
		}
	}
}

func selfSignedCert(t *testing.T, host string) (tls.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		IPAddresses:  []net.IP{net.ParseIP(host)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, key
}

func trustedClientConfig(t *testing.T, cert tls.Certificate) *tls.Config {
	t.Helper()
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}
}

func TestSMTPRejectsUnknownMode(t *testing.T) {
	sender := SMTP{Host: "127.0.0.1", Port: 1, From: "a@b.test", TLSMode: "insecure"}
	if err := sender.Send("to@b.test", "subject", "body"); err == nil {
		t.Fatal("expected error for unknown TLS mode")
	}
}

func TestSMTPRejectsAuthenticatedPlaintext(t *testing.T) {
	sender := SMTP{Host: "127.0.0.1", Port: 1, From: "a@b.test", Username: "u", Password: "p", TLSMode: ModeOff}
	if err := sender.Send("to@b.test", "subject", "body"); err == nil {
		t.Fatal("expected error sending credentials without TLS")
	}
}

func TestSMTPNoHostIsOptional(t *testing.T) {
	sender := SMTP{Host: "", From: "a@b.test", TLSMode: ModeOff}
	if err := sender.Send("to@b.test", "subject", "body"); err != nil {
		t.Fatalf("optional delivery should be a no-op, got %v", err)
	}
}

func TestSMTPStartTLSDelivery(t *testing.T) {
	port, cert := startTestSMTPServer(t, true, false, true)
	sender := SMTP{Host: "127.0.0.1", Port: atoi(port), From: "from@example.test", Username: "user", Password: "pass", TLSMode: ModeStartTLS, DialTimeout: 2 * time.Second, Timeout: 2 * time.Second, TLSConfig: trustedClientConfig(t, cert)}
	if err := sender.Send("to@example.test", "Verify ✓", "hello world"); err != nil {
		t.Fatalf("starttls send: %v", err)
	}
}

func TestSMTPImplicitTLSDelivery(t *testing.T) {
	port, cert := startTestSMTPServer(t, false, true, false)
	sender := SMTP{Host: "127.0.0.1", Port: atoi(port), From: "from@example.test", TLSMode: ModeTLS, DialTimeout: 2 * time.Second, Timeout: 2 * time.Second, TLSConfig: trustedClientConfig(t, cert)}
	if err := sender.Send("to@example.test", "Reset", "body"); err != nil {
		t.Fatalf("implicit tls send: %v", err)
	}
}

func TestSMTPMessageHeadersAreUTF8Capable(t *testing.T) {
	sender := SMTP{Host: "127.0.0.1", Port: 1, From: "from@example.test", TLSMode: ModeOff}
	raw := sender.message("to@example.test", "Verify your account ✓", "secret")
	text := string(raw)
	for _, want := range []string{"MIME-Version: 1.0", "Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: base64", "Subject: =?utf-8?"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing header %q in message:\n%s", want, text)
		}
	}
}

func atoi(value string) int {
	n := 0
	for _, r := range value {
		n = n*10 + int(r-'0')
	}
	return n
}
