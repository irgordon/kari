package services

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/adapters"
	"github.com/irgordon/kari/api/internal/core/domain"
	pb "github.com/irgordon/kari/api/internal/grpc/rustagent"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSiteInput = errors.New("invalid site input")
var ErrSiteBusy = errors.New("site has an operation in progress")

// Sites is the supported static hosting path. It does not expose arbitrary build
// commands or paths from the older, inactive application handlers.
type Sites struct {
	DB       *pgxpool.Pool
	Agent    pb.SystemAgentClient
	ACME     *adapters.AcmeProvider
	PanelURL string
}

type Site struct {
	ID                     uuid.UUID `json:"id"`
	Domain                 string    `json:"domain_name"`
	RepoURL                string    `json:"repo_url"`
	Branch                 string    `json:"branch"`
	Status                 string    `json:"status"`
	SSLStatus              string    `json:"ssl_status"`
	ExpiresAt              time.Time `json:"expires_at"`
	LatestDeploymentID     *string   `json:"latest_deployment_id"`
	LatestDeploymentStatus *string   `json:"latest_deployment_status"`
}

func (s *Sites) Create(ctx context.Context, owner uuid.UUID, hostname, repository, branch string) (*Site, error) {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	panel, _ := url.Parse(s.PanelURL)
	if panel != nil && strings.EqualFold(hostname, panel.Hostname()) {
		return nil, fmt.Errorf("%w: the panel hostname is reserved", ErrSiteInput)
	}
	u, err := url.Parse(repository)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(repository) > 2048 || validator.New().Var(hostname, "required,fqdn,max=253") != nil {
		return nil, fmt.Errorf("%w: use a DNS hostname and public HTTPS Git URL without credentials", ErrSiteInput)
	}
	if branch == "" {
		branch = "main"
	}
	if len(branch) > 100 || strings.HasPrefix(branch, "-") || strings.Contains(branch, "..") || strings.IndexFunc(branch, func(r rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-", r)
	}) >= 0 {
		return nil, fmt.Errorf("%w: invalid branch", ErrSiteInput)
	}
	id, domainID := uuid.New(), uuid.New()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO domains(id,user_id,domain_name,document_root,status) VALUES($1,$2,$3,$4,'provisioning')`, domainID, owner, hostname, "sites/"+id.String()+"/current")
	if err != nil {
		var conflict *pgconn.PgError
		if errors.As(err, &conflict) && conflict.Code == "23505" {
			return nil, fmt.Errorf("%w: hostname is unavailable", ErrSiteInput)
		}
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO applications(id,name,domain_id,app_type,owner_id,app_user,repo_url,branch,build_command,start_command,port) VALUES($1,$2,$3,'static',$4,$5,$6,$7,'','',8080)`, id, hostname, domainID, owner, "ks-"+strings.ReplaceAll(id.String(), "-", "")[:24], repository, branch)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE domains SET app_id=$1 WHERE id=$2`, id, domainID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Site{ID: id, Domain: hostname, RepoURL: repository, Branch: branch, Status: "stopped", SSLStatus: "none", ExpiresAt: time.Unix(0, 0)}, nil
}

// The supported topology has one API worker. After a restart, a host operation
// may have completed without its acknowledgement reaching PostgreSQL. Report
// that uncertainty as failure and permit an explicit republish, never success.
func (s *Sites) RecoverInterrupted(ctx context.Context) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `WITH interrupted AS (
 UPDATE deployments p SET status='failed',updated_at=NOW() FROM applications a
 WHERE p.app_id=a.id AND a.app_type='static' AND p.status='running' RETURNING p.id)
 INSERT INTO deployment_logs(deployment_id,content) SELECT id,'Panel restarted before publication was acknowledged. Inspect the site and publish again.' FROM interrupted`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE domains d SET ssl_status=CASE WHEN expires_at>NOW() THEN 'active' ELSE 'failed' END FROM applications a WHERE d.app_id=a.id AND a.app_type='static' AND d.ssl_status IN ('pending','renewing')`)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const sitesSelect = `SELECT a.id,d.domain_name,a.repo_url,a.branch,a.status,d.ssl_status,d.expires_at,
 (SELECT p.id::text FROM deployments p WHERE p.app_id=a.id ORDER BY p.created_at DESC LIMIT 1),
 (SELECT p.status FROM deployments p WHERE p.app_id=a.id ORDER BY p.created_at DESC LIMIT 1)
 FROM applications a JOIN domains d ON d.id=a.domain_id WHERE a.owner_id=$1 AND d.user_id=$1 AND a.app_type='static'`

func (s *Sites) List(ctx context.Context, owner uuid.UUID) ([]Site, error) {
	rows, err := s.DB.Query(ctx, sitesSelect+` ORDER BY a.created_at DESC LIMIT 100`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]Site, 0)
	for rows.Next() {
		var v Site
		if err = rows.Scan(&v.ID, &v.Domain, &v.RepoURL, &v.Branch, &v.Status, &v.SSLStatus, &v.ExpiresAt, &v.LatestDeploymentID, &v.LatestDeploymentStatus); err != nil {
			return nil, err
		}
		records = append(records, v)
	}
	return records, rows.Err()
}

func (s *Sites) get(ctx context.Context, owner, id uuid.UUID) (*Site, error) {
	var v Site
	err := s.DB.QueryRow(ctx, sitesSelect+` AND a.id=$2`, owner, id).Scan(&v.ID, &v.Domain, &v.RepoURL, &v.Branch, &v.Status, &v.SSLStatus, &v.ExpiresAt, &v.LatestDeploymentID, &v.LatestDeploymentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	}
	return &v, err
}

func (s *Sites) Deploy(ctx context.Context, owner, id uuid.UUID) (string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var hostname, repo, branch, ssl string
	err = tx.QueryRow(ctx, `SELECT d.domain_name,a.repo_url,a.branch,d.ssl_status FROM applications a JOIN domains d ON d.id=a.domain_id WHERE a.id=$1 AND a.owner_id=$2 AND d.user_id=$2 AND a.app_type='static' FOR UPDATE OF a,d`, id, owner).Scan(&hostname, &repo, &branch, &ssl)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var busy bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE app_id=$1 AND status IN ('pending','running'))`, id).Scan(&busy)
	if err != nil {
		return "", err
	}
	if busy || ssl == "pending" || ssl == "renewing" {
		return "", ErrSiteBusy
	}
	deploymentID := uuid.New().String()
	_, err = tx.Exec(ctx, `INSERT INTO deployments(id,app_id,domain_name,repo_url,branch,build_command,target_port,status) VALUES($1,$2,$3,$4,$5,'',8080,'pending')`, deploymentID, id, hostname, repo, branch)
	if err != nil {
		return "", err
	}
	return deploymentID, tx.Commit(ctx)
}

type SiteDeployment struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Logs      []string  `json:"logs"`
}

func (s *Sites) Deployment(ctx context.Context, owner, id, deployment uuid.UUID) (*SiteDeployment, error) {
	var result SiteDeployment
	err := s.DB.QueryRow(ctx, `SELECT p.id,p.status,p.created_at FROM deployments p JOIN applications a ON a.id=p.app_id WHERE p.id=$1 AND a.id=$2 AND a.owner_id=$3`, deployment, id, owner).Scan(&result.ID, &result.Status, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT content FROM deployment_logs WHERE deployment_id=$1 ORDER BY id DESC LIMIT 100`, deployment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result.Logs = make([]string, 0)
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			return nil, err
		}
		result.Logs = append(result.Logs, line)
	}
	for i, j := 0, len(result.Logs)-1; i < j; i, j = i+1, j-1 {
		result.Logs[i], result.Logs[j] = result.Logs[j], result.Logs[i]
	}
	return &result, rows.Err()
}

func (s *Sites) beginTLS(ctx context.Context, owner, id uuid.UUID) (*Site, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var ignored uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM applications WHERE id=$1 AND owner_id=$2 AND app_type='static' FOR UPDATE`, id, owner).Scan(&ignored)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	site, err := s.get(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	var busy bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE app_id=$1 AND status IN ('pending','running'))`, id).Scan(&busy)
	if err != nil {
		return nil, err
	}
	if site.Status != "running" || busy || site.SSLStatus == "pending" || site.SSLStatus == "renewing" {
		return nil, ErrSiteBusy
	}
	_, err = tx.Exec(ctx, `UPDATE domains SET ssl_status='pending' WHERE app_id=$1`, id)
	if err != nil {
		return nil, err
	}
	return site, tx.Commit(ctx)
}

// Certificate issuance is synchronous with a bounded lifetime; no detached
// goroutine reports success before the agent has validated and reloaded Nginx.
func (s *Sites) IssueTLS(ctx context.Context, owner, id uuid.UUID) error {
	site, err := s.beginTLS(ctx, owner, id)
	if err != nil {
		return err
	}
	var email string
	err = s.DB.QueryRow(ctx, `SELECT email FROM users WHERE id=$1 AND email_verified_at IS NOT NULL`, owner).Scan(&email)
	if err == nil && s.ACME != nil {
		var expiry time.Time
		expiry, err = s.ACME.ProvisionSiteCertificate(ctx, id.String(), email, site.Domain)
		return s.finishTLS(id, site, expiry, err)
	}
	if err == nil {
		err = errors.New("ACME provider is unavailable")
	}
	return s.finishTLS(id, site, time.Time{}, err)
}

func (s *Sites) ImportTLS(ctx context.Context, owner, id uuid.UUID, chain, key []byte) error {
	defer clear(key)
	pair, err := tls.X509KeyPair(chain, key)
	if err != nil {
		return fmt.Errorf("%w: certificate and private key do not match", ErrSiteInput)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return ErrSiteInput
	}
	site, err := s.get(ctx, owner, id)
	if err != nil {
		return err
	}
	if cert.VerifyHostname(site.Domain) != nil || time.Now().Before(cert.NotBefore) || !time.Now().Before(cert.NotAfter) || len(chain) > 65536 || len(key) > 16384 {
		return fmt.Errorf("%w: certificate hostname, lifetime, or size is invalid", ErrSiteInput)
	}
	site, err = s.beginTLS(ctx, owner, id)
	if err != nil {
		return err
	}
	reply, err := s.Agent.ActivateSiteTLS(ctx, &pb.SiteCertificate{SiteId: id.String(), DomainName: site.Domain, FullchainPem: chain, PrivkeyPem: key})
	if err == nil && !reply.Success {
		err = errors.New("agent rejected certificate activation")
	}
	return s.finishTLS(id, site, cert.NotAfter, err)
}

func (s *Sites) finishTLS(id uuid.UUID, previous *Site, expiry time.Time, operation error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status := "active"
	if operation != nil {
		status = "failed"
		expiry = previous.ExpiresAt
		if previous.SSLStatus == "active" {
			status = "active"
		}
	}
	_, err := s.DB.Exec(ctx, `UPDATE domains SET ssl_status=$1,expires_at=$2 WHERE app_id=$3`, status, expiry, id)
	if err != nil {
		return err
	}
	if operation != nil {
		return errors.New("TLS operation failed; previous working configuration was preserved. Check DNS, ports 80/443, certificate details, and host logs")
	}
	return nil
}
