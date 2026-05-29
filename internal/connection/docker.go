package connection

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// DockerContainer describes a running database container surfaced at
// GET /api/docker/containers (§6.4). The connection wizard uses it to prefill
// a DSN from a local container.
type DockerContainer struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Image  string   `json:"image"`
	Engine string   `json:"engine"` // "postgres" | "mysql" | "mongodb" | "redis"
	Ports  []string `json:"ports"`  // "hostPort:containerPort/proto"
}

// dbImageEngines maps a substring found in a container image name to the
// canonical engine. Order does not matter: each substring is distinct enough
// that at most one matches a given image.
var dbImageEngines = []struct {
	match  string
	engine string
}{
	{"postgres", "postgres"},
	{"mariadb", "mysql"},
	{"mysql", "mysql"},
	{"mongo", "mongodb"},
	{"redis", "redis"},
}

// engineForImage returns the canonical engine for a container image, or ""
// when the image is not a recognized database.
func engineForImage(image string) string {
	img := strings.ToLower(image)
	for _, m := range dbImageEngines {
		if strings.Contains(img, m.match) {
			return m.engine
		}
	}
	return ""
}

// ListDBContainers returns running containers whose image looks like a database
// engine (postgres, mysql/mariadb, mongo, redis).
//
// A missing or unreachable Docker daemon is NOT an error for this endpoint: the
// wizard simply shows an empty list. In that case it returns (nil, nil).
func ListDBContainers(ctx context.Context) ([]DockerContainer, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		// Could not even construct a client (e.g. no DOCKER_HOST and no socket).
		return nil, nil
	}
	defer func() { _ = cli.Close() }()

	result, err := cli.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		if client.IsErrConnectionFailed(err) {
			// Daemon not running / not reachable: treat as "no containers".
			return nil, nil
		}
		return nil, err
	}

	out := make([]DockerContainer, 0, len(result.Items))
	for _, ctr := range result.Items {
		engine := engineForImage(ctr.Image)
		if engine == "" {
			continue
		}
		out = append(out, DockerContainer{
			ID:     ctr.ID,
			Name:   containerName(ctr.Names),
			Image:  ctr.Image,
			Engine: engine,
			Ports:  containerPorts(ctr.Ports),
		})
	}
	return out, nil
}

// containerName returns the first name with the leading "/" trimmed.
func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

// containerPorts formats published port mappings as "hostPort:containerPort/proto".
// Unpublished ports (PublicPort == 0) are skipped — they cannot be reached from
// the host, so the wizard cannot use them.
func containerPorts(ports []container.PortSummary) []string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.PublicPort == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%d:%d/%s", p.PublicPort, p.PrivatePort, p.Type))
	}
	return out
}
