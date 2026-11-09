package services

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	pb "github.com/irgordon/kari/api/internal/grpc/rustagent"
	"golang.org/x/crypto/ssh"
)

type WorkloadInput struct {
	Action      string            `json:"action"`
	Path        string            `json:"path"`
	Content     []byte            `json:"content"`
	PublicKey   string            `json:"public_key"`
	Environment map[string]string `json:"environment"`
	BackupID    string            `json:"backup_id"`
	Confirm     string            `json:"confirm"`
}

// Every host operation starts from an owner-scoped record. Mutations reserve
// the application row so backup, file, runtime and deployment work cannot race.
func (s *Sites) Workload(ctx context.Context, owner, id uuid.UUID, resource string, input WorkloadInput) (result any, operationErr error) {
	site, err := s.Get(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	allowed := map[string]string{
		"runtime": " status logs start stop restart environment ", "database": " status create delete ",
		"files": " list read write mkdir delete ", "sftp": " status enable disable ", "backups": " list create restore delete ",
	}
	if !strings.Contains(allowed[resource], " "+input.Action+" ") {
		return nil, fmt.Errorf("%w: unsupported workload action", ErrSiteInput)
	}
	mutating := input.Action != "status" && input.Action != "logs" && input.Action != "list" && input.Action != "read"
	if (input.Action == "delete" || input.Action == "restore") && input.Confirm != site.Domain {
		return nil, fmt.Errorf("%w: confirm the website hostname before destructive operations", ErrSiteInput)
	}
	if resource == "runtime" && site.Runtime != "nodejs" {
		return nil, fmt.Errorf("%w: runtime controls require a Node.js website", ErrSiteInput)
	}
	if resource == "sftp" && input.Action == "enable" {
		_, _, options, rest, parseErr := ssh.ParseAuthorizedKey([]byte(input.PublicKey))
		if parseErr != nil || len(options) != 0 || len(strings.TrimSpace(string(rest))) != 0 {
			return nil, fmt.Errorf("%w: supply one public key without SSH options", ErrSiteInput)
		}
	}
	if resource == "runtime" && input.Action == "environment" {
		if len(input.Environment) > 64 {
			return nil, fmt.Errorf("%w: at most 64 variables", ErrSiteInput)
		}
		for key, value := range input.Environment {
			validName := key != ""
			for index, char := range key {
				if !(char >= 'A' && char <= 'Z' || char == '_' || index > 0 && char >= '0' && char <= '9') {
					validName = false
				}
			}
			if !validName || strings.ContainsAny(value, "\n\r\x00") || len(value) > 8192 || key == "PORT" || key == "HOST" || key == "NODE_ENV" || key == "KARI_DATA_DIR" || key == "DATABASE_URL" {
				return nil, fmt.Errorf("%w: use uppercase environment names and single-line values; reserved runtime variables cannot be replaced", ErrSiteInput)
			}
		}
	}
	if mutating {
		tag, err := s.DB.Exec(ctx, `UPDATE applications a SET operation=$1 WHERE a.id=$2 AND a.owner_id=$3 AND a.operation='' AND NOT EXISTS(SELECT 1 FROM deployments p WHERE p.app_id=a.id AND p.status IN ('pending','running')) AND NOT EXISTS(SELECT 1 FROM domains d WHERE d.id=a.domain_id AND d.ssl_status IN ('pending','renewing'))`, resource, id, owner)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() != 1 {
			return nil, ErrSiteBusy
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, releaseErr := s.DB.Exec(cleanup, `UPDATE applications SET operation='' WHERE id=$1`, id)
			if releaseErr != nil && operationErr == nil {
				operationErr = releaseErr
			}
		}()
	}
	switch resource {
	case "runtime":
		result, err := s.Agent.SiteRuntime(ctx, &pb.RuntimeRequest{SiteId: id.String(), DomainName: site.Domain, Action: input.Action, EnvVars: input.Environment})
		if err == nil && (input.Action == "start" || input.Action == "stop" || input.Action == "restart") {
			status := "stopped"
			if result.Running {
				status = "running"
			}
			_, err = s.DB.Exec(ctx, `UPDATE applications SET status=$1 WHERE id=$2`, status, id)
		}
		return result, err
	case "database":
		if input.Action == "create" {
			password := make([]byte, 32)
			if _, err = rand.Read(password); err != nil {
				return nil, err
			}
			defer clear(password)
			tag, err := s.DB.Exec(ctx, `INSERT INTO customer_databases(app_id,status) VALUES($1,'provisioning') ON CONFLICT DO NOTHING`, id)
			if err != nil {
				return nil, err
			}
			if tag.RowsAffected() != 1 {
				return nil, fmt.Errorf("%w: database metadata already exists; inspect or explicitly delete it before retrying", ErrSiteInput)
			}
			result, operationErr := s.Agent.SiteDatabase(ctx, &pb.DatabaseRequest{SiteId: id.String(), DomainName: site.Domain, Action: "create", Password: hex.EncodeToString(password)})
			state := "ready"
			if operationErr != nil {
				state = "failed"
			}
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err = s.DB.Exec(cleanup, `UPDATE customer_databases SET status=$1 WHERE app_id=$2`, state, id); err != nil {
				return nil, err
			}
			return result, operationErr
		}
		result, err := s.Agent.SiteDatabase(ctx, &pb.DatabaseRequest{SiteId: id.String(), DomainName: site.Domain, Action: input.Action})
		if err == nil && input.Action == "delete" {
			_, err = s.DB.Exec(ctx, `DELETE FROM customer_databases WHERE app_id=$1`, id)
		}
		return result, err
	case "files":
		return s.Agent.SiteFiles(ctx, &pb.FileRequest{SiteId: id.String(), DomainName: site.Domain, Action: input.Action, Path: input.Path, Content: input.Content})
	case "sftp":
		return s.Agent.SiteSFTP(ctx, &pb.SFTPRequest{SiteId: id.String(), DomainName: site.Domain, Action: input.Action, PublicKey: input.PublicKey})
	case "backups":
		backup := input.BackupID
		if input.Action == "create" {
			backup = uuid.NewString()
		}
		if input.Action != "list" {
			if _, err = uuid.Parse(backup); err != nil {
				return nil, fmt.Errorf("%w: invalid backup ID", ErrSiteInput)
			}
		}
		result, err := s.Agent.SiteBackups(ctx, &pb.BackupRequest{SiteId: id.String(), DomainName: site.Domain, Action: input.Action, BackupId: backup})
		if err == nil && input.Action == "restore" {
			sslStatus := "none"
			expiry := time.Unix(0, 0)
			if len(result.CertificatePem) > 0 {
				block, _ := pem.Decode(result.CertificatePem)
				if block == nil {
					return nil, fmt.Errorf("restored certificate metadata is invalid")
				}
				cert, parseErr := x509.ParseCertificate(block.Bytes)
				if parseErr != nil {
					return nil, parseErr
				}
				expiry = cert.NotAfter
				sslStatus = "active"
				if !time.Now().Before(expiry) {
					sslStatus = "expired"
				}
			}
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				return nil, err
			}
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, `UPDATE applications SET status='running',repo_url=CASE WHEN $2='' THEN repo_url ELSE $2 END,branch=CASE WHEN $3='' THEN branch ELSE $3 END WHERE id=$1`, id, result.Repository, result.Branch)
			if err != nil {
				return nil, err
			}
			if _, err = tx.Exec(ctx, `UPDATE domains SET status='active',ssl_status=$1,expires_at=$2 WHERE app_id=$3`, sslStatus, expiry, id); err != nil {
				return nil, err
			}
			if result.DatabaseExists {
				_, err = tx.Exec(ctx, `INSERT INTO customer_databases(app_id,status) VALUES($1,'ready') ON CONFLICT(app_id) DO UPDATE SET status='ready'`, id)
			} else {
				_, err = tx.Exec(ctx, `DELETE FROM customer_databases WHERE app_id=$1`, id)
			}
			if err != nil {
				return nil, err
			}
			return result, tx.Commit(ctx)
		}
		return result, err
	}
	return nil, domain.ErrNotFound
}
