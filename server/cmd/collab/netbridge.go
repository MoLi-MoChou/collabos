package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/collabos/collabos/server/internal/daemon/egress"
)

// netbridge runs inside the sandbox: it relays a loopback TCP port to the
// daemon's egress proxy socket and runs the agent command with stdio intact.
func netbridge(args []string) error {
	fs := flag.NewFlagSet("netbridge", flag.ExitOnError)
	sock := fs.String("unix", "", "proxy unix socket")
	listen := fs.String("listen", "127.0.0.1:18080", "loopback address to relay")
	_ = fs.Parse(args)
	argv := fs.Args()
	if *sock == "" || len(argv) == 0 {
		return errors.New("usage: collab netbridge --unix SOCK [--listen ADDR] -- CMD [ARGS...]")
	}
	l, err := net.Listen("tcp", *listen)
	if err != nil {
		return fmt.Errorf("netbridge listen: %w", err)
	}
	go func() { _ = egress.Bridge(l, *sock) }()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	sig := make(chan os.Signal, 4)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for s := range sig {
			_ = cmd.Process.Signal(s)
		}
	}()
	err = cmd.Wait()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		os.Exit(ee.ExitCode())
	}
	return err
}
