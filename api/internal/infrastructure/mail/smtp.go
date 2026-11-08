package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host, Username, Password, From string
	Port                           int
	Development                    bool
}

type SMTP struct {
	config Config
	from   *stdmail.Address
}

func NewSMTP(config Config) (*SMTP, error) {
	if config.Host == "" {
		if config.Username != "" || config.Password != "" || config.From != "" {
			return nil, errors.New("SMTP host is required when email settings are supplied")
		}
		return nil, nil
	}
	if strings.ContainsAny(config.Host+config.From, "\r\n") || config.Port < 1 || config.Port > 65535 {
		return nil, errors.New("invalid SMTP host, port, or sender")
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, errors.New("SMTP username and password must be supplied together")
	}
	from, err := stdmail.ParseAddress(config.From)
	if err != nil {
		return nil, errors.New("SMTP sender must be a valid email address")
	}
	return &SMTP{config: config, from: from}, nil
}

func (s *SMTP) Send(ctx context.Context, recipient, subject, body string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	to, err := stdmail.ParseAddress(recipient)
	if err != nil || strings.ContainsAny(recipient+subject, "\r\n") {
		return errors.New("invalid email recipient or subject")
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port)))
	if err != nil {
		return fmt.Errorf("SMTP connect: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(10 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, s.config.Host)
	if err != nil {
		return fmt.Errorf("SMTP greeting: %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err = client.StartTLS(&tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("SMTP TLS: %w", err)
		}
	} else {
		ip := net.ParseIP(s.config.Host)
		// Plaintext delivery is exclusively for an unauthenticated local development sink.
		if !s.config.Development || ip == nil || !ip.IsLoopback() || s.config.Username != "" {
			return errors.New("SMTP server must support STARTTLS")
		}
	}
	if s.config.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)); err != nil {
			return fmt.Errorf("SMTP authentication: %w", err)
		}
	}
	if err = client.Mail(s.from.Address); err != nil {
		return err
	}
	if err = client.Rcpt(to.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	var message strings.Builder
	fmt.Fprintf(&message, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", s.from.String(), to.String(), mime.QEncoding.Encode("UTF-8", subject), time.Now().Format(time.RFC1123Z))
	encoded := quotedprintable.NewWriter(&message)
	if _, err = encoded.Write([]byte(body)); err != nil {
		return err
	}
	if err = encoded.Close(); err != nil {
		return err
	}
	if _, err = writer.Write([]byte(message.String())); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return fmt.Errorf("SMTP delivery: %w", err)
	}
	return client.Quit()
}
