package dns

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	mdns "github.com/miekg/dns"
)

type Query struct {
	Time      time.Time
	ClientIP  string
	Domain    string
	Type      string
	Responded bool
}

type Sink interface {
	Log(Query)
}

type Server struct {
	addr     string
	upstream []string
	sink     Sink
	log      *slog.Logger

	client    *mdns.Client
	upIdx     uint64
	mu        sync.Mutex
	udpServer *mdns.Server
	tcpServer *mdns.Server
}

func New(addr string, upstream []string, sink Sink, log *slog.Logger) *Server {
	if len(upstream) == 0 {
		upstream = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}
	return &Server{
		addr:     addr,
		upstream: upstream,
		sink:     sink,
		log:      log,
		client:   &mdns.Client{Timeout: 3 * time.Second},
	}
}

func (s *Server) Start(ctx context.Context) error {
	handler := mdns.HandlerFunc(s.handle)

	s.udpServer = &mdns.Server{Addr: s.addr, Net: "udp", Handler: handler}
	s.tcpServer = &mdns.Server{Addr: s.addr, Net: "tcp", Handler: handler}

	errCh := make(chan error, 2)
	go func() { errCh <- s.udpServer.ListenAndServe() }()
	go func() { errCh <- s.tcpServer.ListenAndServe() }()

	s.log.Info("dns server listening", "addr", s.addr, "upstream", s.upstream)

	select {
	case err := <-errCh:
		_ = s.udpServer.Shutdown()
		_ = s.tcpServer.Shutdown()
		return err
	case <-ctx.Done():
		_ = s.udpServer.Shutdown()
		_ = s.tcpServer.Shutdown()
		return ctx.Err()
	}
}

func (s *Server) handle(w mdns.ResponseWriter, req *mdns.Msg) {
	q := Query{
		Time:     time.Now(),
		ClientIP: clientIP(w.RemoteAddr()),
	}
	if len(req.Question) > 0 {
		q.Domain = strings.TrimSuffix(req.Question[0].Name, ".")
		q.Type = mdns.TypeToString[req.Question[0].Qtype]
	}

	upstream := s.pickUpstream()
	resp, _, err := s.client.Exchange(req, upstream)
	if err != nil {
		s.log.Warn("upstream failure", "upstream", upstream, "domain", q.Domain, "err", err)
		q.Responded = false
		s.sink.Log(q)

		fail := new(mdns.Msg)
		fail.SetRcode(req, mdns.RcodeServerFailure)
		_ = w.WriteMsg(fail)
		return
	}

	q.Responded = true
	s.sink.Log(q)
	_ = w.WriteMsg(resp)
}

func (s *Server) pickUpstream() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.upstream[int(s.upIdx)%len(s.upstream)]
	s.upIdx++
	return u
}

func clientIP(a net.Addr) string {
	switch v := a.(type) {
	case *net.UDPAddr:
		return v.IP.String()
	case *net.TCPAddr:
		return v.IP.String()
	default:
		return a.String()
	}
}
