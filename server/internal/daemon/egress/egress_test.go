package egress

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowlist(t *testing.T) {
	a := Allowlist{"opencode.ai", "*.github.com"}
	for h, want := range map[string]bool{
		"opencode.ai": true, "api.opencode.ai": false, "github.com": true, "api.github.com": true,
		"evilgithub.com": false, "example.com": false, "OPENCODE.AI.": true,
	} {
		if got := a.Allows(h); got != want {
			t.Errorf("%s: %v", h, got)
		}
	}
	if !(Allowlist{"*"}).Allows("x.y") {
		t.Error("* allows all")
	}
}

func TestHostOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r.git": "github.com", "https://tok@gitlab.example:8443/o/r": "gitlab.example",
		"git@github.com:o/r.git": "github.com", "/srv/git/r.git": "", "ssh://git@host.io:22/r": "host.io",
	} {
		if got := HostOf(in); got != want {
			t.Errorf("%s: %q", in, got)
		}
	}
}

// End to end over a unix socket and the TCP bridge, like inside bwrap.
func TestProxyThroughBridge(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "hello") }))
	defer up.Close()
	upURL, _ := url.Parse(up.URL)
	sock := filepath.Join(t.TempDir(), "p.sock")
	ul, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var events []Decision
	p := &Proxy{Allow: Allowlist{"127.0.0.1"}, OnEvent: func(d Decision) { events = append(events, d) }}
	go func() { _ = p.Serve(ul) }()
	tl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = Bridge(tl, sock) }()
	proxyURL, _ := url.Parse("http://" + tl.Addr().String())
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	res, err := client.Get(up.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(b) != "hello" {
		t.Fatalf("allowed plain http: %q", b)
	}
	// CONNECT to an allowed host tunnels bytes.
	c, err := net.Dial("tcp", tl.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(c, "CONNECT "+upURL.Host+" HTTP/1.1\r\nHost: "+upURL.Host+"\r\n\r\n")
	br := bufio.NewReader(c)
	line, _ := br.ReadString('\n')
	if !strings.Contains(line, "200") {
		t.Fatalf("connect: %q", line)
	}
	c.Close()
	// A host outside the allowlist is refused.
	res, err = client.Get("http://localhost:" + upURL.Port())
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("blocked: %d", res.StatusCode)
	}
	blocked := false
	for _, e := range events {
		if !e.Allowed {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("blocked decision not reported")
	}
}
