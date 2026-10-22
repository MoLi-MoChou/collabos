// Command collab is CollabOS v2: one binary for the relay server, the device
// daemon and the MCP server that lets a coding agent call agents on other
// devices.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/collabos/collabos/v2/internal/config"
	"github.com/collabos/collabos/v2/internal/daemon"
	"github.com/collabos/collabos/v2/internal/gitctx"
	"github.com/collabos/collabos/v2/internal/mcp"
	"github.com/collabos/collabos/v2/internal/server"
)

const usage = `collab — let coding agents on different machines call each other

Device setup:
  collab login --server URL --token TOKEN     save the relay URL and this device's token
  collab agent NAME | -- CMD ARGS...          ACP agent that answers incoming calls
                                              (opencode [default], claude, codex, gemini, grok, or a command)
  collab model PROVIDER/MODEL                 model for opencode (optional)
  collab repo add PATH [--remote URL]         local clone callers' patches are applied to
  collab status                               show config
  collab daemon                               connect and answer incoming calls (foreground)
  collab mcp                                  stdio MCP server for your agent (list_agents, call_agent, ...)

Manual use:
  collab agents                               list devices
  collab call TARGET "PROMPT" [--cwd DIR] [--no-code]   call a remote agent and print the reply
  collab calls                                recent calls

Server:
  collab server [--addr :8080]                run the relay (DATABASE_URL=postgres://... ; --mem for in-memory)
  collab server add-device NAME --owner OWNER print a new token for a device (DATABASE_URL)

Environment: COLLAB_HOME (default ~/.collab), COLLAB_SERVER, COLLAB_TOKEN
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	args := os.Args[2:]
	switch os.Args[1] {
	case "server":
		err = runServer(ctx, args)
	case "login":
		err = login(args)
	case "agent":
		err = setAgent(args)
	case "model":
		err = edit(func(c *config.Config) error {
			if len(args) != 1 {
				return fmt.Errorf("usage: collab model PROVIDER/MODEL")
			}
			c.Model = args[0]
			return nil
		})
	case "repo":
		err = repo(args)
	case "status":
		err = status()
	case "daemon", "run":
		err = runDaemon(ctx)
	case "mcp":
		err = runMCP(ctx)
	case "agents":
		err = agents(ctx)
	case "call":
		err = call(ctx, args)
	case "calls":
		err = calls(ctx)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "collab:", err)
		os.Exit(1)
	}
}

func logger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }

func runServer(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "add-device" {
		fs := flag.NewFlagSet("add-device", flag.ExitOnError)
		owner := fs.String("owner", "", "owner name, e.g. kai")
		if len(args) < 2 {
			return fmt.Errorf("usage: collab server add-device NAME --owner OWNER")
		}
		_ = fs.Parse(args[2:])
		if *owner == "" {
			return fmt.Errorf("--owner is required")
		}
		st, err := server.OpenPG(ctx, os.Getenv("DATABASE_URL"))
		if err != nil {
			return err
		}
		defer st.Close()
		tok, hash := server.NewToken()
		if err := st.AddDevice(ctx, args[1], *owner, hash); err != nil {
			return err
		}
		fmt.Println(tok)
		return nil
	}
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	mem := fs.Bool("mem", false, "in-memory store (testing)")
	_ = fs.Parse(args)
	var st server.Store
	if *mem {
		m := server.NewMemStore()
		// COLLAB_DEVICES="name:owner:token,..." seeds devices for local testing.
		for _, d := range strings.Split(os.Getenv("COLLAB_DEVICES"), ",") {
			if p := strings.SplitN(d, ":", 3); len(p) == 3 {
				_ = m.AddDevice(ctx, p[0], p[1], server.HashToken(p[2]))
			}
		}
		st = m
	} else {
		pg, err := server.OpenPG(ctx, os.Getenv("DATABASE_URL"))
		if err != nil {
			return err
		}
		defer pg.Close()
		st = pg
	}
	s := server.New(st, logger())
	srv := &http.Server{Addr: *addr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	s.Log.Info("relay listening", "addr", *addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func edit(fn func(*config.Config) error) error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	if err := fn(&c); err != nil {
		return err
	}
	if err := config.Save(c); err != nil {
		return err
	}
	fmt.Println("saved", config.Path())
	return nil
}

func login(args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	srv := fs.String("server", "", "relay URL, e.g. http://host:8190")
	tok := fs.String("token", "", "device token (cdv_...)")
	_ = fs.Parse(args)
	if *srv == "" || !strings.HasPrefix(*tok, "cdv_") {
		return fmt.Errorf("--server and a cdv_ --token are required")
	}
	return edit(func(c *config.Config) error {
		c.Server, c.Token = strings.TrimRight(*srv, "/"), *tok
		return nil
	})
}

func setAgent(args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: collab agent opencode|claude|codex|gemini|grok  or  collab agent -- CMD ARGS")
	}
	return edit(func(c *config.Config) error { c.Agent = args; return nil })
}

func repo(args []string) error {
	if len(args) < 2 || args[0] != "add" {
		return fmt.Errorf("usage: collab repo add PATH [--remote URL]")
	}
	fs := flag.NewFlagSet("repo add", flag.ExitOnError)
	remote := fs.String("remote", "", "remote URL (default: origin)")
	_ = fs.Parse(args[2:])
	path, err := filepath.Abs(args[1])
	if err != nil {
		return err
	}
	if *remote == "" {
		if *remote, err = gitctx.Git(context.Background(), path, nil, "remote", "get-url", "origin"); err != nil {
			return err
		}
	}
	return edit(func(c *config.Config) error {
		if c.Repos == nil {
			c.Repos = map[string]string{}
		}
		c.Repos[*remote] = path
		return nil
	})
}

func status() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	tok := "(none)"
	if len(c.Token) > 10 {
		tok = c.Token[:8] + "…"
	}
	fmt.Printf("config: %s\nserver: %s\ntoken:  %s\nagent:  %s\nmodel:  %s\n", config.Path(), c.Server, tok, strings.Join(c.AgentArgv(), " "), c.Model)
	for r, p := range c.Repos {
		fmt.Printf("repo:   %s -> %s\n", r, p)
	}
	if c.Server != "" && c.Token != "" {
		ds, err := api(c).Agents(context.Background())
		if err != nil {
			fmt.Println("relay:  error:", err)
		} else {
			fmt.Print("devices:\n", mcp.FormatAgents(ds))
		}
	}
	return nil
}

func api(c config.Config) *mcp.API { return &mcp.API{Server: c.Server, Token: c.Token} }

func loadAPI() (*mcp.API, error) {
	c, err := config.Load()
	if err != nil {
		return nil, err
	}
	if c.Server == "" || c.Token == "" {
		return nil, fmt.Errorf("not configured: run `collab login --server URL --token TOKEN` (config %s)", config.Path())
	}
	return api(c), nil
}

func runDaemon(ctx context.Context) error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	return daemon.New(c, logger()).Serve(ctx)
}

func runMCP(ctx context.Context) error {
	a, err := loadAPI()
	if err != nil {
		return err
	}
	wait := 30 * time.Minute
	if v, err := time.ParseDuration(os.Getenv("COLLAB_MCP_MAX_WAIT")); err == nil && v > 0 {
		wait = v
	}
	s := &mcp.Server{API: a, MaxWait: wait, Cwd: mcp.Cwd()}
	return s.Serve(ctx, os.Stdin, os.Stdout)
}

func agents(ctx context.Context) error {
	a, err := loadAPI()
	if err != nil {
		return err
	}
	ds, err := a.Agents(ctx)
	if err != nil {
		return err
	}
	fmt.Print(mcp.FormatAgents(ds))
	return nil
}

func call(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: collab call TARGET \"PROMPT\" [--cwd DIR] [--no-code] [--timeout 30m]")
	}
	fs := flag.NewFlagSet("call", flag.ExitOnError)
	cwd := fs.String("cwd", "", "repository to share")
	noCode := fs.Bool("no-code", false, "do not attach git context")
	timeout := fs.Duration("timeout", 30*time.Minute, "remote work limit")
	_ = fs.Parse(args[2:])
	a, err := loadAPI()
	if err != nil {
		return err
	}
	s := &mcp.Server{API: a, MaxWait: *timeout + time.Minute, Cwd: mcp.Cwd()}
	inc := !*noCode
	out, err := s.CallAgent(ctx, args[0], args[1], *cwd, inc, timeout.Minutes(), func(m string) { fmt.Fprintln(os.Stderr, "…", m) })
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func calls(ctx context.Context) error {
	a, err := loadAPI()
	if err != nil {
		return err
	}
	cs, err := a.List(ctx, 20)
	if err != nil {
		return err
	}
	for _, c := range cs {
		took := ""
		if c.FinishedAt != nil {
			took = c.FinishedAt.Sub(c.CreatedAt).Round(time.Second).String()
		}
		fmt.Printf("%s  %s/%s -> %s/%s  %-9s %6s  %s\n", c.ID, c.FromOwner, c.FromDevice, c.ToOwner, c.ToDevice, c.Status, took, oneLine(c.Prompt, 60))
	}
	return nil
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

