package obs

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// A struct mirroring a connection config, to confirm field-name redaction.
type connCfg struct {
	Host     string
	Password string
}

func (c connCfg) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("host", c.Host),
		slog.String("password", c.Password),
	)
}

func TestRedactsSensitiveKeys(t *testing.T) {
	var buf bytes.Buffer
	log := Setup(slog.LevelDebug, &buf, false)

	log.Info("connecting",
		"password", "secret123",
		"token", "abc.def.ghi",
		"host", "localhost",
	)

	out := buf.String()
	if strings.Contains(out, "secret123") {
		t.Fatalf("password leaked into logs: %s", out)
	}
	if strings.Contains(out, "abc.def.ghi") {
		t.Fatalf("token leaked into logs: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] marker, got: %s", out)
	}
	if !strings.Contains(out, "localhost") {
		t.Fatalf("non-sensitive host should survive, got: %s", out)
	}
}

func TestRedactsDSNValue(t *testing.T) {
	var buf bytes.Buffer
	log := Setup(slog.LevelDebug, &buf, false)

	// Key "endpoint" is not in sensitiveKeys, so this exercises value-shape detection.
	log.Info("opening", "endpoint", "postgresql://app:hunter2@db.internal:5432/myapp")

	out := buf.String()
	if strings.Contains(out, "hunter2") {
		t.Fatalf("DSN password leaked: %s", out)
	}
	if !strings.Contains(out, "postgresql://app:***@db.internal:5432/myapp") {
		t.Fatalf("expected scrubbed DSN, got: %s", out)
	}
}

func TestRedactsNestedStructPassword(t *testing.T) {
	var buf bytes.Buffer
	log := Setup(slog.LevelDebug, &buf, false)

	log.Info("config", "conn", connCfg{Host: "localhost", Password: "topsecret"})

	out := buf.String()
	if strings.Contains(out, "topsecret") {
		t.Fatalf("nested password leaked: %s", out)
	}
}
