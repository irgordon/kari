//go:build integration

package contract_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/core/services"
)

func TestStaticSitePersistenceAndOwnership(t *testing.T) {
	suite := newContractSuite(t)
	t.Cleanup(suite.pool.Close)
	suite.reset(t)
	owner := insertUser(t, suite.pool, superadminRoleID, "site-owner")
	other := insertUser(t, suite.pool, superadminRoleID, "other-owner")
	sites := &services.Sites{DB: suite.pool, PanelURL: "https://panel.example.test"}
	ctx := context.Background()
	_, err := sites.Create(ctx, owner, "panel.example.test", "https://github.com/example/site.git", "main", "static")
	if !errors.Is(err, services.ErrSiteInput) {
		t.Fatal("panel hostname was not reserved")
	}
	site, err := sites.Create(ctx, owner, "www.example.test", "https://github.com/example/site.git", "main", "static")
	requireNoError(t, err)
	_, err = sites.Create(ctx, owner, "www.example.test", "https://github.com/example/site.git", "main", "static")
	if !errors.Is(err, services.ErrSiteInput) {
		t.Fatalf("duplicate site: %v", err)
	}
	_, err = sites.Create(ctx, owner, "another.example.test", "https://user:password@github.com/example/site.git", "main", "static")
	if !errors.Is(err, services.ErrSiteInput) {
		t.Fatalf("embedded credentials accepted: %v", err)
	}
	records, err := sites.List(ctx, owner)
	requireNoError(t, err)
	if len(records) != 1 || records[0].ID != site.ID {
		t.Fatal("created site not returned")
	}
	records, err = sites.List(ctx, other)
	requireNoError(t, err)
	if len(records) != 0 {
		t.Fatal("other owner can list site")
	}
	_, err = sites.Deploy(ctx, other, site.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-owner deploy: %v", err)
	}
	id, err := sites.Deploy(ctx, owner, site.ID)
	requireNoError(t, err)
	_, err = sites.Deploy(ctx, owner, site.ID)
	if !errors.Is(err, services.ErrSiteBusy) {
		t.Fatalf("duplicate queued deployment: %v", err)
	}
	_, err = sites.Deployment(ctx, other, site.ID, uuid.MustParse(id))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-owner log read: %v", err)
	}
	claimed, err := suite.deployments.ClaimNextPending(ctx)
	requireNoError(t, err)
	if claimed == nil || claimed.ID != id {
		t.Fatal("queue did not claim real site")
	}
	requireNoError(t, suite.deployments.AppendLog(ctx, id, "Published static site"))
	requireNoError(t, suite.deployments.UpdateStatus(ctx, id, domain.StatusSuccess))
	result, err := sites.Deployment(ctx, owner, site.ID, uuid.MustParse(id))
	requireNoError(t, err)
	if result.Status != "success" || len(result.Logs) != 1 {
		t.Fatalf("incorrect persisted publication: %#v", result)
	}
	// A failed replacement must not falsely report success or hide the existing site.
	id, err = sites.Deploy(ctx, owner, site.ID)
	requireNoError(t, err)
	_, err = suite.deployments.ClaimNextPending(ctx)
	requireNoError(t, err)
	requireNoError(t, suite.deployments.UpdateStatus(ctx, id, domain.StatusFailed))
	records, err = sites.List(ctx, owner)
	requireNoError(t, err)
	if records[0].Status != "running" {
		t.Fatal("failed replacement hid the previous successful site")
	}
	result, err = sites.Deployment(ctx, owner, site.ID, uuid.MustParse(id))
	requireNoError(t, err)
	if result.Status != "failed" {
		t.Fatal("failed replacement reported success")
	}
	id, err = sites.Deploy(ctx, owner, site.ID)
	requireNoError(t, err)
	_, err = suite.deployments.ClaimNextPending(ctx)
	requireNoError(t, err)
	requireNoError(t, sites.RecoverInterrupted(ctx))
	result, err = sites.Deployment(ctx, owner, site.ID, uuid.MustParse(id))
	requireNoError(t, err)
	if result.Status != "failed" || len(result.Logs) != 1 {
		t.Fatal("interrupted publication was not reported")
	}
	_, err = sites.Deploy(ctx, owner, site.ID)
	requireNoError(t, err)
	node, err := sites.Create(ctx, owner, "node.example.test", "https://github.com/example/node.git", "main", "nodejs")
	requireNoError(t, err)
	stored, err := sites.Get(ctx, owner, node.ID)
	requireNoError(t, err)
	if stored.Runtime != "nodejs" {
		t.Fatal("runtime was not persisted")
	}
	_, err = sites.Workload(ctx, other, node.ID, "database", services.WorkloadInput{Action: "create"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("cross-owner workload operation was accepted")
	}
	_, err = sites.Workload(ctx, owner, node.ID, "backups", services.WorkloadInput{Action: "restore", BackupID: uuid.NewString(), Confirm: "wrong.example.test"})
	if !errors.Is(err, services.ErrSiteInput) {
		t.Fatal("restore without hostname confirmation was accepted")
	}
	_, err = sites.Workload(ctx, owner, node.ID, "sftp", services.WorkloadInput{Action: "enable", PublicKey: "command=evil ssh-ed25519 invalid"})
	if !errors.Is(err, services.ErrSiteInput) {
		t.Fatal("SFTP authorization options were accepted")
	}
	_, err = suite.pool.Exec(ctx, `UPDATE applications SET operation='backups' WHERE id=$1`, node.ID)
	requireNoError(t, err)
	_, err = sites.Deploy(ctx, owner, node.ID)
	if !errors.Is(err, services.ErrSiteBusy) {
		t.Fatal("publication raced a workload operation")
	}
}
