// Command migrate applies or reverts the embedded SQL migrations.
//
//	migrate up | down | version
package main

import (
	"fmt"
	"os"

	"github.com/collabos/collabos/server/migrations"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "up":
		err = migrations.Up(url)
	case "down":
		err = migrations.Down(url)
	case "version":
		v, dirty, e := migrations.Version(url)
		err = e
		if e == nil {
			fmt.Printf("version=%d dirty=%v\n", v, dirty)
		}
	default:
		err = fmt.Errorf("unknown command %q (up|down|version)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	fmt.Println("migrate", cmd, "ok")
}
