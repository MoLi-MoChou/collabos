package server

import (
	"testing"

	"github.com/collabos/collabos/v2/internal/wire"
)

func TestResolve(t *testing.T) {
	ds := []wire.Device{
		{Name: "kai-pc", Owner: "kai", Online: true},
		{Name: "zs-old", Owner: "zhangsan"},
		{Name: "zs-box", Owner: "zhangsan", Online: true},
	}
	if d, _ := Resolve(ds, "@ZhangSan", "kai-pc"); d.Name != "zs-box" {
		t.Fatal(d)
	}
	if d, _ := Resolve(ds, "zs-old", "kai-pc"); d.Name != "zs-old" {
		t.Fatal(d)
	}
	if _, err := Resolve(ds, "kai", "kai-pc"); err == nil {
		t.Fatal("caller's own device should not be picked by owner")
	}
}
