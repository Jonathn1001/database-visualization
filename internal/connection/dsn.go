package connection

import (
	"net/url"
	"strings"

	"github.com/elgnas/dbviz/internal/model"
)

// DetectEngine resolves the engine for a connection config. An explicit
// cfg.Engine wins; otherwise it is inferred from the DSN scheme or file path.
func DetectEngine(cfg model.ConnectionConfig) (string, error) {
	if cfg.Engine != "" {
		return strings.ToLower(cfg.Engine), nil
	}
	if cfg.FilePath != "" {
		return "sqlite", nil
	}
	if cfg.DSN != "" {
		return engineFromDSN(cfg.DSN)
	}
	return "", model.NewAPIError(model.ErrConnInvalidDSN, "cannot determine engine").
		WithHint("provide an engine, a DSN, or a file path")
}

func engineFromDSN(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", model.NewAPIError(model.ErrConnInvalidDSN, "malformed DSN")
	}
	switch strings.ToLower(u.Scheme) {
	case "postgres", "postgresql":
		return "postgres", nil
	case "mongodb", "mongodb+srv":
		return "mongodb", nil
	case "mysql":
		return "mysql", nil
	case "file", "sqlite", "sqlite3":
		return "sqlite", nil
	default:
		return "", model.NewAPIError(model.ErrConnInvalidDSN, "unsupported DSN scheme: "+u.Scheme)
	}
}
