// Package egress is the daemon-side network gate for sandboxed agents
// (PRD 7.2.2 / 11.x). The sandbox has no network of its own; the agent's
// HTTP(S) traffic goes through this proxy, which only connects to allowlisted
// hosts (by default the model API and the project's Git remote host).
package egress

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Allowlist matches host names. "example.com" matches exactly; "*.example.com"
// or ".example.com" matches any subdomain (and the apex).
type Allowlist []string

// Allows reports whether host (without port) may be reached.
func (a Allowlist) Allows(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	for _, p := range a {
		p = strings.ToLower(strings.TrimSpace(p))
		switch {
		case p == "":
		case p == "*":
			return true
		case strings.HasPrefix(p, "*.") || strings.HasPrefix(p, "."):
			base := strings.TrimLeft(p, "*.")
			if h == base || strings.HasSuffix(h, "."+base) {
				return true
			}
		case h == p:
			return true
		}
	}
	return false
}

// Decision is reported for every connection attempt.
type Decision struct {
	Host    string
	Port    string
	Allowed bool
}

// Proxy is a minimal HTTP proxy supporting CONNECT and absolute-URI requests.
type Proxy struct {
	Allow   Allowlist
	OnEvent func(Decision)
	Dial    func(ctx context.Context, network, addr string) (net.Conn, error)

	mu   sync.Mutex
	seen map[string]bool
}

func (p *Proxy) report(d Decision) {
	if p.OnEvent == nil {
		return
	}
	// Report each host/outcome once per proxy to keep task logs readable.
	key := d.Host + ":" + d.Port + map[bool]string{true: "+", false: "-"}[d.Allowed]
	p.mu.Lock()
	if p.seen == nil {
		p.seen = map[string]bool{}
	}
	dup := p.seen[key]
	p.seen[key] = true
	p.mu.Unlock()
	if !dup {
		p.OnEvent(d)
	}
}

func (p *Proxy) dial(ctx context.Context, addr string) (net.Conn, error) {
	if p.Dial != nil {
		return p.Dial(ctx, "tcp", addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

// Serve accepts connections until the listener is closed.
func (p *Proxy) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go p.handle(c)
	}
}

func splitHost(hostport, defPort string) (string, string) {
	h, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return strings.Trim(hostport, "[]"), defPort
	}
	return h, port
}

func (p *Proxy) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	if req.Method == http.MethodConnect {
		host, port := splitHost(req.Host, "443")
		ok := p.Allow.Allows(host)
		p.report(Decision{host, port, ok})
		if !ok {
			_, _ = io.WriteString(c, "HTTP/1.1 403 Forbidden\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\nCollabOS egress policy: host not allowed: "+host+"\n")
			return
		}
		up, err := p.dial(context.Background(), net.JoinHostPort(host, port))
		if err != nil {
			_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n")
			return
		}
		defer up.Close()
		_, _ = io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\n")
		pipe(c, br, up)
		return
	}
	// Plain HTTP via absolute URI.
	if req.URL == nil || req.URL.Host == "" {
		_, _ = io.WriteString(c, "HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n")
		return
	}
	host, port := splitHost(req.URL.Host, "80")
	ok := p.Allow.Allows(host)
	p.report(Decision{host, port, ok})
	if !ok {
		_, _ = io.WriteString(c, "HTTP/1.1 403 Forbidden\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\nCollabOS egress policy: host not allowed: "+host+"\n")
		return
	}
	up, err := p.dial(context.Background(), net.JoinHostPort(host, port))
	if err != nil {
		_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n")
		return
	}
	defer up.Close()
	req.RequestURI = ""
	req.Header.Del("Proxy-Connection")
	req.Close = true
	if err := req.Write(up); err != nil {
		return
	}
	_, _ = io.Copy(c, up)
}

func pipe(c net.Conn, br *bufio.Reader, up net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(up, br); closeWrite(up); done <- struct{}{} }()
	go func() { _, _ = io.Copy(c, up); closeWrite(c); done <- struct{}{} }()
	<-done
	<-done
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

// HostOf extracts the host of a Git remote URL ("" for local paths).
func HostOf(remote string) string {
	r := strings.TrimSpace(remote)
	if i := strings.Index(r, "://"); i >= 0 {
		rest := r[i+3:]
		if j := strings.IndexAny(rest, "/"); j >= 0 {
			rest = rest[:j]
		}
		if k := strings.LastIndex(rest, "@"); k >= 0 {
			rest = rest[k+1:]
		}
		h, _ := splitHost(rest, "")
		return h
	}
	// scp-like git@host:path
	if at := strings.Index(r, "@"); at >= 0 {
		if colon := strings.Index(r[at:], ":"); colon > 0 {
			return r[at+1 : at+colon]
		}
	}
	return ""
}
