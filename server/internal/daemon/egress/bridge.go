package egress

import (
	"io"
	"net"
)

// Bridge forwards TCP connections accepted on l to the unix socket at sock.
// It runs inside the sandbox's private network namespace, where the only
// way out is the proxy's socket bind-mounted from the daemon.
func Bridge(l net.Listener, sock string) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer c.Close()
			u, err := net.Dial("unix", sock)
			if err != nil {
				return
			}
			defer u.Close()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(u, c); closeWrite(u); done <- struct{}{} }()
			go func() { _, _ = io.Copy(c, u); closeWrite(c); done <- struct{}{} }()
			<-done
			<-done
		}()
	}
}
