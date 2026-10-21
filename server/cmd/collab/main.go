// Command collab is the CollabOS local daemon and its setup CLI.
//
//	collab login  --server URL --token NODE_TOKEN
//	collab repo add PATH [--remote URL]
//	collab repo list
//	collab status
//	collab run
//
// COLLAB_HOME selects the config/journal directory (default ~/.collabos),
// which lets several daemons run side by side on one machine.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/collabos/collabos/server/internal/daemon/agent"
	"github.com/collabos/collabos/server/internal/daemon/config"
	"github.com/collabos/collabos/server/internal/daemon/worktree"
)

func usage() {
	fmt.Fprint(os.Stderr, `collab — CollabOS daemon

  collab login --server URL --token NODE_TOKEN   save server and node token
  collab repo add PATH [--remote URL]            map a local checkout to its project remote
  collab repo list                               list mapped repositories
  collab status                                  show config and detected capabilities
  collab run                                     connect and execute tasks (foreground)

Environment: COLLAB_HOME (default ~/.collabos)
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "login":
		err = login(os.Args[2:])
	case "repo":
		err = repo(os.Args[2:])
	case "status":
		err = status()
	case "run":
		err = run()
	case "netbridge":
		err = netbridge(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "collab:", err)
		os.Exit(1)
	}
}

func login(args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	server := fs.String("server", "", "server URL, e.g. http://localhost:8080")
	token := fs.String("token", "", "node token from the pairing dialog (cos_n_...)")
	_ = fs.Parse(args)
	if *server == "" || !strings.HasPrefix(*token, "cos_n_") {
		return fmt.Errorf("--server and a cos_n_ --token are required")
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	c.ServerURL, c.NodeToken = strings.TrimRight(*server, "/"), *token
	if err := config.Save(c); err != nil {
		return err
	}
	fmt.Println("saved", config.Path())
	return nil
}

func repo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: collab repo add PATH [--remote URL] | collab repo list")
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		for _, r := range c.Repos {
			fmt.Printf("%s\t%s\n", r.RemoteURL, r.Path)
		}
		return nil
	case "add":
		fs := flag.NewFlagSet("repo add", flag.ExitOnError)
		remote := fs.String("remote", "", "project remote URL (default: git remote get-url origin)")
		if len(args) < 2 {
			return fmt.Errorf("usage: collab repo add PATH [--remote URL]")
		}
		_ = fs.Parse(args[2:])
		path, err := filepath.Abs(args[1])
		if err != nil {
			return err
		}
		if *remote == "" {
			*remote, err = worktree.Git(context.Background(), path, "remote", "get-url", "origin")
			if err != nil {
				return fmt.Errorf("cannot read origin of %s: %w", path, err)
			}
		}
		c.AddRepo(config.Repo{RemoteURL: *remote, Path: path})
		if err := config.Save(c); err != nil {
			return err
		}
		fmt.Printf("mapped %s -> %s\n", *remote, path)
		return nil
	}
	return fmt.Errorf("unknown repo command %q", args[0])
}

func status() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	tok := ""
	if c.NodeToken != "" {
		tok = c.NodeToken[:10] + "…"
	}
	fmt.Printf("home:   %s\nserver: %s\ntoken:  %s\n", config.Home(), c.ServerURL, tok)
	d, err := agent.New(c)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(d.Capabilities(), "", "  ")
	fmt.Println(string(b))
	return nil
}

func run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	d, err := agent.New(c)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return d.Run(ctx)
}
