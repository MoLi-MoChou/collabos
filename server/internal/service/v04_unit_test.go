package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/collabos/collabos/server/pkg/protocol"
)

func TestValidateRichWhitelist(t *testing.T) {
	ok := protocol.RichUI{Lang: "openui", Source: `root = Stack([Text("a <b> Iframe(x)"), Button("ok", "ack")])`}
	if err := ValidateRich(ok); err != nil {
		t.Fatalf("valid source refused: %v", err)
	}
	for _, src := range []string{
		`root = Iframe("x")`,
		`root = Text(x) <script>alert(1)</script>`,
		`root = Link(https://evil.example)`,
		"",
	} {
		if ValidateRich(protocol.RichUI{Lang: "openui", Source: src}) == nil {
			t.Errorf("accepted %q", src)
		}
	}
	if ValidateRich(protocol.RichUI{Lang: "html", Source: `root = Text("a")`}) == nil {
		t.Error("non-openui lang accepted")
	}
}

func TestExtractRich(t *testing.T) {
	ui, text := ExtractRich(nil, "hello\n```openui\nroot = Text(\"x\")\n```\nbye")
	if ui == nil || strings.Contains(text, "openui") || !strings.Contains(text, "hello") {
		t.Fatalf("fence: %+v %q", ui, text)
	}
	meta := &protocol.RichUI{Lang: "openui", Source: `root = Text("m")`}
	if ui, _ := ExtractRich(meta, "plain"); ui == nil || ui.Source != meta.Source {
		t.Fatal("_meta ignored")
	}
	// Extraction does not validate; attachRich does, so invalid fences never render.
	if ui, _ := ExtractRich(nil, "```openui\nroot = Evil()\n```"); ui == nil || ValidateRich(*ui) == nil {
		t.Fatal("invalid fence must be extracted then refused")
	}
}

func TestApprovalCategory(t *testing.T) {
	if ApprovalCategory(" migrations/0002_x.up.sql | 3 +") != CategoryDBMigration {
		t.Fatal("migration not detected")
	}
	if ApprovalCategory(" src/a.ts | 1 +") == CategoryDBMigration {
		t.Fatal("plain code flagged")
	}
}

func TestGitHubSignature(t *testing.T) {
	body := []byte(`{"a":1}`)
	sig := SignGitHub("k", body)
	if !VerifyGitHubSignature("k", body, sig) || VerifyGitHubSignature("k2", body, sig) || VerifyGitHubSignature("k", body, "") || VerifyGitHubSignature("", body, sig) {
		t.Fatal("signature check wrong")
	}
}

func TestIMPayload(t *testing.T) {
	for kind, key := range map[string]string{"slack": "text", "feishu": "msg_type", "dingtalk": "msgtype"} {
		b, _ := json.Marshal(IMPayload(kind, "hi"))
		if !strings.Contains(string(b), `"`+key+`"`) || !strings.Contains(string(b), "hi") {
			t.Errorf("%s payload: %s", kind, b)
		}
	}
}
