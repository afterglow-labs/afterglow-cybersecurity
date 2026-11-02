package proxygraph

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestASNDiverseAndEmptyPool(t *testing.T) {
	p := NewPool()
	if _, err := p.Next(context.Background(), ASNDiverse); err == nil {
		t.Fatal("empty configured pool must fail closed")
	}
	p.Add(Proxy{ID: "a", URL: "http://127.0.0.1:1001", ASN: "AS1", Active: true})
	p.Add(Proxy{ID: "b", URL: "http://127.0.0.1:1002", ASN: "AS2", Active: true})
	first, err := p.Next(context.Background(), ASNDiverse)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Next(context.Background(), ASNDiverse)
	if err != nil {
		t.Fatal(err)
	}
	if first.ASN == second.ASN {
		t.Fatalf("ASN diversity failed: %q then %q", first.ID, second.ID)
	}
	p.Remove(first.ID)
	p.Remove(second.ID)
	if _, err := p.Next(context.Background(), RoundRobin); err == nil {
		t.Fatal("removed pool must fail closed")
	}
}

func TestHealthChecksOnlyConfiguredListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local sockets unavailable: %v", err)
	}
	defer listener.Close()
	accepted := make(chan int, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			accepted <- -1
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 1)
		n, _ := conn.Read(buf)
		accepted <- n
	}()
	p := NewPool()
	p.Add(Proxy{ID: "local", URL: "http://" + listener.Addr().String(), Active: false})
	got := p.Check(context.Background())
	if len(got) != 1 || !got[0].Active || got[0].Score <= 0 {
		t.Fatalf("healthy listener was not marked active: %+v", got)
	}
	if n := <-accepted; n != 0 {
		t.Fatalf("health probe sent %d bytes; expected connection only", n)
	}
}
