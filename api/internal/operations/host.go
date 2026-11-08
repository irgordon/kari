package operations

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	karidb "github.com/irgordon/kari/api/internal/db"
	"github.com/joho/godotenv"
	"golang.org/x/mod/semver"
)

var bundleFiles = []string{"kari-admin", "kari-agent", "images.tar", "docker-compose.yml", "toolchain.env", "kari-agent.service", "kari-panel.service", "source.manifest", "frontend.conf"}

type Manifest struct {
	Version    string            `json:"version"`
	Arch       string            `json:"arch"`
	Schema     int               `json:"schema"`
	SourceTree string            `json:"source_tree"`
	Files      map[string]string `json:"files"`
}
type Host struct{ Root string }

func HashFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("expected regular file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func CreateManifest(dir, version, arch, tree string) error {
	if !semver.IsValid("v"+version) || (arch != "amd64" && arch != "arm64") || len(tree) != 40 {
		return errors.New("invalid release version, architecture, or source tree")
	}
	plan, err := karidb.LoadMigrationPlan()
	if err != nil {
		return err
	}
	m := Manifest{Version: version, Arch: arch, Schema: len(plan), SourceTree: tree, Files: map[string]string{}}
	for _, name := range bundleFiles {
		hash, err := HashFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		m.Files[name] = hash
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), append(data, '\n'), 0644)
}

func ReadManifest(dir, expectedHash string) (Manifest, error) {
	var m Manifest
	hash, err := HashFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, err
	}
	if expectedHash != "" && hash != expectedHash {
		return m, errors.New("release manifest checksum does not match the trusted checksum")
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, err
	}
	dataHash := sha256.Sum256(data)
	if hex.EncodeToString(dataHash[:]) != hash {
		return m, errors.New("manifest changed while being inspected")
	}
	if len(data) > 65536 {
		return m, errors.New("manifest too large")
	}
	if err = json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	if !semver.IsValid("v"+m.Version) || m.Arch != runtime.GOARCH || len(m.Files) != len(bundleFiles) {
		return m, errors.New("unsupported release version, architecture, or manifest")
	}
	for _, name := range bundleFiles {
		hash, err := HashFile(filepath.Join(dir, name))
		if err != nil {
			return m, err
		}
		if hash != m.Files[name] {
			return m, fmt.Errorf("release file checksum mismatch: %s", name)
		}
	}
	return m, nil
}

func randomSecret() (string, error) {
	data := make([]byte, 32)
	_, err := rand.Read(data)
	return hex.EncodeToString(data), err
}

func InitEnvironment(path, publicURL string, development bool) error {
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return errors.New("environment file already exists or cannot be inspected")
	}
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("public URL must be an absolute origin")
	}
	if u.Scheme != "https" && !(development && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")) {
		return errors.New("production requires an HTTPS public URL")
	}
	if !development && u.Port() != "" && u.Port() != "443" {
		return errors.New("production panel HTTPS uses port 443")
	}
	values := map[string]string{"KARI_ENV": "production", "KARI_REQUIRE_AGENT": "true", "KARI_PUBLIC_URL": strings.TrimRight(publicURL, "/"), "CORS_ALLOWED_ORIGINS": strings.TrimRight(publicURL, "/"), "KARI_AGENT_SOCKET_DIR": "/run/kari", "KARI_API_PORT": "127.0.0.1:8080", "KARI_FRONTEND_PORT": "127.0.0.1:3000"}
	if development {
		values["KARI_ENV"] = "development"
		values["KARI_REQUIRE_AGENT"] = "false"
		values["KARI_AGENT_SOCKET_DIR"] = "./.runtime/agent"
	}
	for _, key := range []string{"DB_PASSWORD", "JWT_SECRET", "ENCRYPTION_KEY", "KARI_SETUP_TOKEN"} {
		value, err := randomSecret()
		if err != nil {
			return err
		}
		values[key] = value
	}
	return writeEnvironment(path, values)
}

func writeEnvironment(path string, values map[string]string) error {
	data, err := godotenv.Marshal(values)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".env-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.WriteString(data + "\n")
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (h Host) CheckRoot() error {
	if os.Geteuid() != 0 || runtime.GOOS != "linux" {
		return errors.New("this host operation requires root on Ubuntu 24.04")
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "ID=ubuntu") || !strings.Contains(string(data), `VERSION_ID="24.04"`) {
		return errors.New("only Ubuntu 24.04 is supported")
	}
	if !filepath.IsAbs(h.Root) || filepath.Clean(h.Root) != h.Root || h.Root == "/" || strings.ContainsAny(h.Root, " \t\r\n;\\\"$") {
		return errors.New("installation root must be a clean absolute directory without shell or unit-file syntax")
	}
	for path := h.Root; path != "/"; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
			return errors.New("installation path contains an unsafe directory")
		}
	}
	return nil
}

func (h Host) Environment() (map[string]string, error) {
	path := filepath.Join(h.Root, ".env.production")
	if err := PrivateFile(path); err != nil {
		return nil, err
	}
	return godotenv.Read(path)
}

func PrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("secret file must be root-owned, regular, and mode 0600")
	}
	return nil
}

func command(ctx context.Context, environment map[string]string, input io.Reader, output io.Writer, binary string, args ...string) error {
	cmd := exec.CommandContext(ctx, binary, args...)
	for _, key := range []string{"PATH", "HOME", "LANG", "LC_ALL", "TZ"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Env = append(cmd.Env, "DOCKER_HOST=unix:///var/run/docker.sock")
	cmd.Stdin = input
	cmd.Stdout = output
	// Child tools may echo configuration. Report the operation and exit status,
	// not arbitrary stderr that could contain credentials.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w", binary, strings.Join(args[:min(1, len(args))], " "), err)
	}
	return nil
}

func (h Host) Compose(ctx context.Context, release string, environment map[string]string, input io.Reader, output io.Writer, args ...string) error {
	base := []string{"compose", "--project-name", "kari", "--env-file", filepath.Join(release, "toolchain.env"), "--env-file", filepath.Join(h.Root, ".env.production"), "-f", filepath.Join(release, "docker-compose.yml")}
	return command(ctx, environment, input, output, "docker", append(base, args...)...)
}

func (h Host) Current() (string, Manifest, error) {
	current, err := filepath.EvalSymlinks(filepath.Join(h.Root, "current"))
	if err != nil {
		return "", Manifest{}, err
	}
	if filepath.Dir(current) != filepath.Join(h.Root, "releases") {
		return "", Manifest{}, errors.New("current release escapes installation root")
	}
	m, err := ReadManifest(current, "")
	return current, m, err
}

func copyFile(src, dst string, mode os.FileMode) error {
	if _, err := HashFile(src); err != nil {
		return err
	}
	if info, err := os.Lstat(dst); err == nil && !info.Mode().IsRegular() {
		return errors.New("destination is not a regular file")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}

func (h Host) stage(bundle string, m Manifest) (string, error) {
	dest := filepath.Join(h.Root, "releases", m.Version)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "", err
	}
	if err := os.Mkdir(dest, 0755); err != nil {
		return "", err
	}
	for _, name := range append(append([]string{}, bundleFiles...), "manifest.json") {
		mode := os.FileMode(0644)
		if name == "kari-admin" || name == "kari-agent" {
			mode = 0755
		}
		if err := copyFile(filepath.Join(bundle, name), filepath.Join(dest, name), mode); err != nil {
			return "", err
		}
	}
	for _, name := range bundleFiles {
		hash, err := HashFile(filepath.Join(dest, name))
		if err != nil {
			return "", err
		}
		if hash != m.Files[name] {
			return "", errors.New("bundle changed during protected staging")
		}
	}
	return dest, nil
}

func (h Host) activate(release string) error {
	tmp := filepath.Join(h.Root, ".current-next")
	if err := os.Symlink(release, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(h.Root, "current")); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func runtimeValues(values map[string]string, m Manifest) {
	values["KARI_API_IMAGE"] = "kari-api:" + m.Version
	values["KARI_FRONTEND_IMAGE"] = "kari-frontend:" + m.Version
	values["KARI_VERSION"] = m.Version
}

func (h Host) Install(ctx context.Context, bundle, expected, publicURL, cert, key string) error {
	if err := h.CheckRoot(); err != nil {
		return err
	}
	if expected == "" {
		return errors.New("supply the trusted release manifest SHA-256")
	}
	m, err := ReadManifest(bundle, expected)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(h.Root); !os.IsNotExist(err) {
		return errors.New("installation root already exists; use upgrade")
	}
	for _, binary := range []string{"docker", "nginx", "systemctl", "getent", "groupadd", "useradd"} {
		if _, err = exec.LookPath(binary); err != nil {
			return fmt.Errorf("missing prerequisite %s", binary)
		}
	}
	if err = command(ctx, nil, nil, io.Discard, "docker", "compose", "version"); err != nil {
		return err
	}
	for _, path := range []string{"/etc/kari/agent.env", "/etc/systemd/system/kari-agent.service", "/etc/systemd/system/kari-panel.service", "/etc/nginx/sites-available/kari-panel.conf", "/etc/nginx/sites-enabled/kari-panel.conf", "/usr/local/libexec/kari/kari-agent"} {
		if _, err = os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("refusing to overwrite existing installation file %s", path)
		}
	}
	if err = PrivateFile(key); err != nil {
		return err
	}
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return errors.New("invalid TLS certificate/key pair")
	}
	u, err := url.Parse(publicURL)
	if err != nil {
		return err
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return err
	}
	if err = leaf.VerifyHostname(u.Hostname()); err != nil || time.Now().After(leaf.NotAfter) || time.Now().Before(leaf.NotBefore) {
		return errors.New("TLS certificate is expired or does not match panel hostname")
	}
	if err = command(ctx, nil, nil, io.Discard, "getent", "passwd", "1001"); err == nil {
		return errors.New("UID 1001 is already allocated; use a dedicated supported host")
	}
	if err = command(ctx, nil, nil, io.Discard, "getent", "group", "1001"); err == nil {
		return errors.New("GID 1001 is already allocated")
	}
	if err = os.Mkdir(h.Root, 0700); err != nil {
		return err
	}
	if err = InitEnvironment(filepath.Join(h.Root, ".env.production"), publicURL, false); err != nil {
		return err
	}
	release, err := h.stage(bundle, m)
	if err != nil {
		return err
	}
	values, err := h.Environment()
	if err != nil {
		return err
	}
	runtimeValues(values, m)
	values["KARI_FRONTEND_CONFIG"] = filepath.Join(h.Root, "frontend.conf")
	if err = writeEnvironment(filepath.Join(h.Root, ".env.production"), values); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "groupadd", "--gid", "1001", "kari-api"); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "useradd", "--uid", "1001", "--gid", "kari-api", "--system", "--no-create-home", "--shell", "/usr/sbin/nologin", "kari-api"); err != nil {
		return err
	}
	if err = os.MkdirAll("/usr/local/libexec/kari", 0755); err != nil {
		return err
	}
	if err = h.activate(release); err != nil {
		return err
	}
	if err = os.Symlink(filepath.Join(h.Root, "current", "kari-agent"), "/usr/local/libexec/kari/kari-agent"); err != nil {
		return err
	}
	if err = os.MkdirAll("/etc/kari", 0700); err != nil {
		return err
	}
	if err = godotenv.Write(map[string]string{"KARI_API_UID": "1001", "KARI_API_GID": "1001", "KARI_SOCKET_PATH": "/run/kari/agent.sock", "KARI_WEB_ROOT": "/var/www/kari", "KARI_SYSTEMD_DIR": "/etc/systemd/system", "KARI_SSL_DIR": "/etc/kari/ssl", "KARI_PROXY_CONF_DIR": "/etc/nginx", "RUST_LOG": "info"}, "/etc/kari/agent.env"); err != nil {
		return err
	}
	if err = os.Chmod("/etc/kari/agent.env", 0600); err != nil {
		return err
	}
	if err = h.refreshUnits(ctx, release); err != nil {
		return err
	}
	if err = os.Mkdir(filepath.Join(h.Root, "tls"), 0700); err != nil {
		return err
	}
	if err = copyFile(cert, filepath.Join(h.Root, "tls/panel.crt"), 0644); err != nil {
		return err
	}
	if err = copyFile(key, filepath.Join(h.Root, "tls/panel.key"), 0600); err != nil {
		return err
	}
	frontend, err := os.ReadFile(filepath.Join(release, "frontend.conf"))
	if err != nil {
		return err
	}
	frontendConfig := strings.Replace(string(frontend), "server {", "server {\n    set_real_ip_from 172.30.231.1;\n    real_ip_header X-Real-IP;", 1)
	if err = os.WriteFile(filepath.Join(h.Root, "frontend.conf"), []byte(frontendConfig), 0644); err != nil {
		return err
	}
	if strings.ContainsAny(u.Hostname(), "\r\n {};\\\"") {
		return errors.New("invalid hostname")
	}
	nginx := fmt.Sprintf("server { listen 443 ssl; server_name %s; ssl_certificate %s/tls/panel.crt; ssl_certificate_key %s/tls/panel.key; location / { proxy_pass http://127.0.0.1:3000; proxy_set_header Host $http_host; proxy_set_header X-Real-IP $remote_addr; proxy_set_header X-Forwarded-Proto https; } }\n", u.Hostname(), h.Root, h.Root)
	if err = os.WriteFile("/etc/nginx/sites-available/kari-panel.conf", []byte(nginx), 0644); err != nil {
		return err
	}
	if err = os.Symlink("/etc/nginx/sites-available/kari-panel.conf", "/etc/nginx/sites-enabled/kari-panel.conf"); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "nginx", "-t"); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "docker", "load", "--input", filepath.Join(release, "images.tar")); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "systemctl", "enable", "--now", "kari-agent.service"); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "systemctl", "reload", "nginx"); err != nil {
		return err
	}
	return command(ctx, nil, nil, io.Discard, "systemctl", "enable", "--now", "kari-panel.service")
}

func (h Host) Start(ctx context.Context) error {
	if err := h.CheckRoot(); err != nil {
		return err
	}
	release, _, err := h.Current()
	if err != nil {
		return err
	}
	values, err := h.Environment()
	if err != nil {
		return err
	}
	if err = h.Compose(ctx, release, values, nil, io.Discard, "up", "--no-build", "--detach"); err != nil {
		return err
	}
	return h.waitReady(ctx, values["KARI_PUBLIC_URL"])
}

func (h Host) waitReady(ctx context.Context, publicURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	roots, err := x509.SystemCertPool()
	if err != nil {
		return err
	}
	certificate, err := os.ReadFile(filepath.Join(h.Root, "tls/panel.crt"))
	if err != nil {
		return err
	}
	if !roots.AppendCertsFromPEM(certificate) {
		return errors.New("invalid installed panel certificate")
	}
	plain := &http.Client{Timeout: 2 * time.Second}
	secure := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, "127.0.0.1:443")
	}}}
	for {
		ready := true
		for _, probe := range []struct {
			client *http.Client
			url    string
		}{{plain, "http://127.0.0.1:8080/ready"}, {plain, "http://127.0.0.1:3000/api/v1/setup/status"}, {secure, publicURL + "/api/v1/setup/status"}} {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.url, nil)
			if err != nil {
				return err
			}
			response, err := probe.client.Do(request)
			if err != nil {
				ready = false
				break
			}
			response.Body.Close()
			if response.StatusCode != 200 {
				ready = false
				break
			}
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("panel readiness failed; inspect agent, API, frontend, and HTTPS proxy health")
		case <-time.After(time.Second):
		}
	}
}

func (h Host) Stop(ctx context.Context) error {
	if err := h.CheckRoot(); err != nil {
		return err
	}
	release, _, err := h.Current()
	if err != nil {
		return err
	}
	values, err := h.Environment()
	if err != nil {
		return err
	}
	return h.Compose(ctx, release, values, nil, io.Discard, "stop", "api", "frontend")
}

func (h Host) Backup(ctx context.Context) (string, error) {
	if err := h.CheckRoot(); err != nil {
		return "", err
	}
	release, m, err := h.Current()
	if err != nil {
		return "", err
	}
	values, err := h.Environment()
	if err != nil {
		return "", err
	}
	var schema bytes.Buffer
	if err = h.Compose(ctx, release, values, nil, &schema, "exec", "--no-TTY", "db", "psql", "--username=kari", "--dbname=kari", "--no-align", "--tuples-only", "--command", "SELECT COALESCE(MAX(version),0) FROM schema_migrations"); err != nil {
		return "", err
	}
	if strings.TrimSpace(schema.String()) != fmt.Sprint(m.Schema) {
		return "", errors.New("installed release and database schema do not match; resolve before backing up")
	}
	if err = h.Stop(ctx); err != nil {
		return "", err
	}
	dir := filepath.Join(h.Root, "backups", time.Now().UTC().Format("20060102T150405.000000000"))
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(filepath.Join(dir, "database.dump"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	err = h.Compose(ctx, release, values, nil, file, "exec", "--no-TTY", "db", "pg_dump", "--username=kari", "--dbname=kari", "--format=custom")
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	for name, source := range map[string]string{"environment": filepath.Join(h.Root, ".env.production"), "agent.env": "/etc/kari/agent.env", "panel.crt": filepath.Join(h.Root, "tls/panel.crt"), "panel.key": filepath.Join(h.Root, "tls/panel.key"), "panel.conf": "/etc/nginx/sites-available/kari-panel.conf", "frontend.conf": filepath.Join(h.Root, "frontend.conf")} {
		if err = copyFile(source, filepath.Join(dir, name), 0600); err != nil {
			return "", err
		}
	}
	proof := karidb.BackupProof{Version: m.Version, SchemaVersion: m.Schema, Files: map[string]string{}}
	for _, name := range []string{"database.dump", "environment", "agent.env", "panel.crt", "panel.key", "panel.conf", "frontend.conf"} {
		hash, err := HashFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		proof.Files[name] = hash
	}
	data, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(dir, "backup.json"), data, 0600); err != nil {
		return "", err
	}
	return dir, nil
}

func (h Host) Upgrade(ctx context.Context, bundle, expected string) (string, error) {
	if err := h.CheckRoot(); err != nil {
		return "", err
	}
	current, old, err := h.Current()
	if err != nil {
		return "", err
	}
	if expected == "" {
		return "", errors.New("a trusted release manifest checksum is required")
	}
	m, err := ReadManifest(bundle, expected)
	if err != nil {
		return "", err
	}
	if semver.Compare("v"+m.Version, "v"+old.Version) <= 0 || m.Schema < old.Schema {
		return "", errors.New("downgrades and reinstallation are unsupported; restore a matching backup instead")
	}
	values, err := h.Environment()
	if err != nil {
		return "", err
	}
	release, err := h.stage(bundle, m)
	if err != nil {
		return "", err
	}
	if err = command(ctx, nil, nil, io.Discard, "docker", "load", "--input", filepath.Join(release, "images.tar")); err != nil {
		return "", err
	}
	runtimeValues(values, m)
	if err = h.Compose(ctx, release, values, nil, io.Discard, "run", "--rm", "--no-deps", "migrate", "--check"); err != nil {
		return "", err
	}
	backup, err := h.Backup(ctx)
	if err != nil {
		return backup, err
	}
	if err = h.Compose(ctx, release, values, nil, io.Discard, "run", "--rm", "--no-deps", "--user", "0:0", "--volume", backup+":/backup:ro", "--env", "KARI_MIGRATION_BACKUP=/backup/backup.json", "migrate"); err != nil {
		return backup, fmt.Errorf("upgrade stopped safely; restore %s before restarting the old release: %w", backup, err)
	}
	if err = writeEnvironment(filepath.Join(h.Root, ".env.production"), values); err != nil {
		return backup, err
	}
	if err = h.activate(release); err != nil {
		return backup, err
	}
	if err = h.refreshUnits(ctx, release); err != nil {
		return backup, err
	}
	if err = command(ctx, nil, nil, io.Discard, "systemctl", "restart", "kari-agent.service"); err != nil {
		return backup, err
	}
	if err = command(ctx, nil, nil, io.Discard, "systemctl", "restart", "kari-panel.service"); err != nil {
		return backup, fmt.Errorf("new release failed readiness; restore %s (previous release %s): %w", backup, current, err)
	}
	if err = h.waitReady(ctx, values["KARI_PUBLIC_URL"]); err != nil {
		return backup, err
	}
	return backup, nil
}

func (h Host) Restore(ctx context.Context, backup string) error {
	if err := h.CheckRoot(); err != nil {
		return err
	}
	resolved, pathErr := filepath.EvalSymlinks(backup)
	if pathErr != nil || resolved != backup || !strings.HasPrefix(backup, filepath.Join(h.Root, "backups")+string(os.PathSeparator)) {
		return errors.New("restore requires a backup imported into the protected backups directory without symlinks")
	}
	if err := PrivateFile(filepath.Join(backup, "backup.json")); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(backup, "backup.json"))
	if err != nil {
		return err
	}
	var proof karidb.BackupProof
	if err = json.Unmarshal(data, &proof); err != nil {
		return err
	}
	if !semver.IsValid("v"+proof.Version) || len(proof.Files) != 7 {
		return errors.New("invalid backup manifest")
	}
	for _, name := range []string{"database.dump", "environment", "agent.env", "panel.crt", "panel.key", "panel.conf", "frontend.conf"} {
		if proof.Files[name] == "" {
			return errors.New("backup manifest is incomplete")
		}
	}
	for name, expected := range proof.Files {
		if filepath.Base(name) != name {
			return errors.New("unsafe backup file name")
		}
		hash, err := HashFile(filepath.Join(backup, name))
		if err != nil {
			return err
		}
		if hash != expected {
			return errors.New("backup checksum mismatch")
		}
	}
	release := filepath.Join(h.Root, "releases", proof.Version)
	restored, err := ReadManifest(release, "")
	if err != nil {
		return err
	}
	if restored.Schema != proof.SchemaVersion {
		return errors.New("backup schema and release disagree")
	}
	values, err := godotenv.Read(filepath.Join(backup, "environment"))
	if err != nil {
		return err
	}
	if err = h.Compose(ctx, release, values, nil, io.Discard, "stop", "api", "frontend"); err != nil {
		return err
	}
	file, err := os.Open(filepath.Join(backup, "database.dump"))
	if err != nil {
		return err
	}
	defer file.Close()
	if err = h.Compose(ctx, release, values, file, io.Discard, "exec", "--no-TTY", "db", "pg_restore", "--username=kari", "--dbname=postgres", "--clean", "--if-exists", "--create", "--exit-on-error"); err != nil {
		return err
	}
	// A snapshot can predate revocation or one-time-code consumption. Never
	// reactivate those capabilities when rolling application data back.
	cleanup := `BEGIN; UPDATE users SET credential_version=credential_version+1,
		mfa_pending_secret=NULL,mfa_pending_until=NULL,
		mfa_last_counter=GREATEST(mfa_last_counter,FLOOR(EXTRACT(EPOCH FROM NOW())/30)::bigint);
		UPDATE auth_sessions SET revoked_at=NOW();
		UPDATE account_email_tokens SET consumed_at=NOW(); DELETE FROM mfa_recovery_codes; COMMIT;`
	if err = h.Compose(ctx, release, values, strings.NewReader(cleanup), io.Discard, "exec", "--no-TTY", "db", "psql", "--username=kari", "--dbname=kari", "--set=ON_ERROR_STOP=1"); err != nil {
		return err
	}
	values["JWT_SECRET"], err = randomSecret()
	if err != nil {
		return err
	}
	if err = writeEnvironment(filepath.Join(h.Root, ".env.production"), values); err != nil {
		return err
	}
	for name, dest := range map[string]string{"agent.env": "/etc/kari/agent.env", "panel.crt": filepath.Join(h.Root, "tls/panel.crt"), "panel.key": filepath.Join(h.Root, "tls/panel.key"), "panel.conf": "/etc/nginx/sites-available/kari-panel.conf", "frontend.conf": filepath.Join(h.Root, "frontend.conf")} {
		mode := os.FileMode(0600)
		if name == "panel.crt" || name == "panel.conf" || name == "frontend.conf" {
			mode = 0644
		}
		if err = replaceFile(filepath.Join(backup, name), dest, mode); err != nil {
			return err
		}
	}
	if err = command(ctx, nil, nil, io.Discard, "nginx", "-t"); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "systemctl", "reload", "nginx"); err != nil {
		return err
	}
	if err = h.activate(release); err != nil {
		return err
	}
	if err = h.refreshUnits(ctx, release); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "systemctl", "restart", "kari-agent.service"); err != nil {
		return err
	}
	if err = command(ctx, nil, nil, io.Discard, "systemctl", "restart", "kari-panel.service"); err != nil {
		return err
	}
	return h.waitReady(ctx, values["KARI_PUBLIC_URL"])
}

func replaceFile(src, dst string, mode os.FileMode) error {
	if _, err := HashFile(src); err != nil {
		return err
	}
	if info, err := os.Lstat(dst); err == nil && !info.Mode().IsRegular() {
		return errors.New("unsafe restore destination")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dst), ".kari-restore-*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	if err = out.Chmod(mode); err == nil {
		_, err = io.Copy(out, in)
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(out.Name(), dst)
}

func (h Host) refreshUnits(ctx context.Context, release string) error {
	if err := replaceFile(filepath.Join(release, "kari-agent.service"), "/etc/systemd/system/kari-agent.service", 0644); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(release, "kari-panel.service"))
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(h.Root, ".panel-unit-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.WriteString(strings.ReplaceAll(string(data), "/opt/kari", h.Root))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = replaceFile(file.Name(), "/etc/systemd/system/kari-panel.service", 0644); err != nil {
		return err
	}
	return command(ctx, nil, nil, io.Discard, "systemctl", "daemon-reload")
}
