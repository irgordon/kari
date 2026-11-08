package mail

import (
	"context"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
)

func TestSMTPRefusesTLSStripping(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	commands := make(chan string, 2)
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		protocol := textproto.NewConn(conn)
		protocol.PrintfLine("220 local SMTP")
		if _, err = protocol.ReadLine(); err != nil {
			return
		}
		protocol.PrintfLine("250 localhost")
		if command, err := protocol.ReadLine(); err == nil {
			commands <- command
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	sender, err := NewSMTP(Config{Host: host, Port: portNumber, From: "panel@example.test", Username: "operator", Password: "must-not-be-sent"})
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.Send(context.Background(), "owner@example.test", "Reset", "secret-link"); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("TLS stripping was not rejected: %v", err)
	}
	<-done
	select {
	case command := <-commands:
		t.Fatalf("sent a command after TLS refusal: %s", command)
	default:
	}
}

func TestSMTPRejectsHeaderInjection(t *testing.T) {
	if _, err := NewSMTP(Config{Host: "127.0.0.1", Port: 25, From: "panel@example.test\r\nBcc: victim@example.test"}); err == nil {
		t.Fatal("sender header injection accepted")
	}
	sender, err := NewSMTP(Config{Host: "127.0.0.1", Port: 25, From: "panel@example.test", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.Send(context.Background(), "owner@example.test\r\nBcc: victim@example.test", "Reset", "link"); err == nil {
		t.Fatal("recipient header injection accepted")
	}
}
