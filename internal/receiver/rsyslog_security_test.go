package receiver

import (
	"net"
	"testing"
	"time"

	"fwlog/internal/model"
)

func TestTCPReceiverClosesIdleConnection(t *testing.T) {
	oldTimeout := tcpIdleTimeout
	tcpIdleTimeout = 50 * time.Millisecond
	t.Cleanup(func() { tcpIdleTimeout = oldTimeout })

	port := freeTCPPort(t)
	manager := NewManager()
	t.Cleanup(manager.Close)
	if err := manager.ApplySources([]model.LogSource{{
		SourceID:       "idle-test",
		Enabled:        true,
		SourceType:     "rsyslog",
		ListenProtocol: "tcp",
		ListenHost:     "127.0.0.1",
		ListenPort:     port,
		ClientIP:       "127.0.0.1",
		SpoolDir:       t.TempDir(),
	}}); err != nil {
		t.Fatalf("ApplySources: %v", err)
	}

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", intToString(port)))
	if err != nil {
		t.Fatalf("dial receiver: %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	one := make([]byte, 1)
	if _, err := conn.Read(one); err == nil {
		t.Fatal("空闲 TCP 连接未在超时后关闭")
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatalf("连接由客户端读超时结束，而不是服务端空闲超时关闭: %v", err)
	}
}
