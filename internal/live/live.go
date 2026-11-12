// Package live inspects the running deployment: containers, the shared Docker
// network, MySQL databases and HTTP endpoints.
package live

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Container is the subset of `docker compose ps --format json` we use.
type Container struct {
	Service string `json:"Service"`
	Name    string `json:"Name"`
	State   string `json:"State"`
	Health  string `json:"Health"`
	Status  string `json:"Status"`
}

// DockerAvailable reports whether the Docker daemon answers.
func DockerAvailable(ctx context.Context) bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return exec.CommandContext(c, "docker", "info", "--format", "{{.ServerVersion}}").Run() == nil
}

func run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.String(), fmt.Errorf("%s %s: %w%s", name, strings.Join(args, " "), err, suffix(msg))
	}
	return out.String(), nil
}

func suffix(msg string) string {
	if msg == "" {
		return ""
	}
	return ": " + msg
}

// ComposePS lists containers in a compose project, including stopped ones.
func ComposePS(ctx context.Context, dir string) ([]Container, error) {
	out, err := run(ctx, dir, "docker", "compose", "ps", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parsePS(out)
}

func parsePS(out string) ([]Container, error) {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var cs []Container
		if err := json.Unmarshal([]byte(trimmed), &cs); err != nil {
			return nil, err
		}
		return cs, nil
	}
	var cs []Container
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var c Container
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, err
		}
		cs = append(cs, c)
	}
	return cs, nil
}

// NetworkExists checks the shared external Docker network.
func NetworkExists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	_, err := run(ctx, "", "docker", "network", "inspect", name)
	if err != nil {
		if strings.Contains(err.Error(), "No such network") || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Databases lists schemas from the infra MySQL container.
func Databases(ctx context.Context, infraDir string) ([]string, error) {
	out, err := run(ctx, infraDir, "docker", "compose", "exec", "-T", "mysql", "sh", "-c",
		`exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -N -B -e 'SHOW DATABASES'`)
	if err != nil {
		return nil, err
	}
	var dbs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "Database") {
			continue
		}
		dbs = append(dbs, line)
	}
	return dbs, nil
}

// HTTPStatus performs a GET and returns the status code.
func HTTPStatus(ctx context.Context, url string) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "emobase-cli/0.1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
