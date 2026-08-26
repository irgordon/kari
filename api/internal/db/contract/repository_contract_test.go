//go:build integration

package contract_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/irgordon/kari/api/internal/core/domain"
	karidb "github.com/irgordon/kari/api/internal/db"
	"github.com/irgordon/kari/api/internal/db/postgres"
)

const (
	superadminRoleID = "00000000-0000-0000-0000-000000000001"
	viewerRoleID     = "00000000-0000-0000-0000-000000000004"
	profileID        = "20000000-0000-0000-0000-000000000001"
)

type contractSuite struct {
	pool         *pgxpool.Pool
	users        *postgres.UserRepo
	applications domain.ApplicationRepository
	domains      *postgres.DomainRepository
	deployments  *postgres.PostgresDeploymentRepository
	audits       domain.AuditRepository
	profiles     *karidb.PostgresProfileRepository
}

func TestRepositoryContracts(t *testing.T) {
	suite := newContractSuite(t)
	t.Cleanup(suite.pool.Close)
	tests := []struct {
		name string
		run  func(*testing.T, *contractSuite)
	}{
		{"schema head", testSchemaHead},
		{"users", testUserRepository},
		{"domains", testDomainRepository},
		{"applications", testApplicationRepository},
		{"audits", testAuditRepository},
		{"deployments", testDeploymentRepository},
		{"profiles", testProfileRepository},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suite.reset(t)
			test.run(t, suite)
		})
	}
}

func newContractSuite(t *testing.T) *contractSuite {
	t.Helper()
	databaseURL := os.Getenv("KARI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("KARI_TEST_DATABASE_URL is required for integration contracts")
	}
	pool, err := postgres.NewPool(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open contract database: %v", err)
	}
	return &contractSuite{
		pool: pool, users: postgres.NewUserRepo(pool),
		applications: postgres.NewApplicationRepo(pool), domains: postgres.NewDomainRepository(pool),
		deployments: postgres.NewPostgresDeploymentRepository(pool), audits: postgres.NewAuditRepository(pool),
		profiles: karidb.NewPostgresProfileRepository(pool),
	}
}

func (suite *contractSuite) reset(t *testing.T) {
	t.Helper()
	const truncateSQL = `
		TRUNCATE deployment_logs, deployments, applications,
		         domains, system_alerts, users RESTART IDENTITY CASCADE`
	const profileSQL = `
		INSERT INTO system_profiles (
			id, default_stack_registry, ssl_strategy, max_memory_per_app_mb,
			max_cpu_percent_per_app, default_firewall_policy,
			app_user_uid_range_start, app_user_uid_range_end, backup_retention_days
		) VALUES ($1, '{"nodejs":"node:22"}', 'manual', 512, 50,
		          'deny-by-default', 20000, 29999, 30)`
	context := context.Background()
	requireNoError(t, execStatement(context, suite.pool, truncateSQL))
	requireNoError(t, execStatement(context, suite.pool, "DELETE FROM system_profiles"))
	_, err := suite.pool.Exec(context, profileSQL, profileID)
	requireNoError(t, err)
}

func execStatement(ctx context.Context, pool *pgxpool.Pool, statement string) error {
	_, err := pool.Exec(ctx, statement)
	return err
}

func testSchemaHead(t *testing.T, suite *contractSuite) {
	assertCount(t, suite.pool, "SELECT COUNT(*) FROM schema_migrations", 4)
	assertCount(t, suite.pool, "SELECT COUNT(*) FROM roles", 4)
	assertCount(t, suite.pool, "SELECT COUNT(*) FROM permissions", 9)
	assertCount(t, suite.pool, "SELECT COUNT(*) FROM pg_extension WHERE extname = 'pgcrypto'", 1)
	assertPermissionVocabulary(t, suite.pool)
}

func assertPermissionVocabulary(t *testing.T, pool *pgxpool.Pool) {
	const query = `SELECT resource || ':' || action FROM permissions ORDER BY resource, action`
	rows, err := pool.Query(context.Background(), query)
	requireNoError(t, err)
	defer rows.Close()
	var observed []string
	for rows.Next() {
		var permission string
		requireNoError(t, rows.Scan(&permission))
		observed = append(observed, permission)
	}
	expected := []string{
		"applications:delete", "applications:deploy", "applications:read", "applications:write",
		"audit_logs:read", "domains:delete", "domains:read", "domains:write", "server:manage",
	}
	if fmt.Sprint(observed) != fmt.Sprint(expected) {
		t.Fatalf("permission vocabulary = %v, want %v", observed, expected)
	}
}

func testUserRepository(t *testing.T, suite *contractSuite) {
	userID := insertUser(t, suite.pool, superadminRoleID, "admin")
	assertUserReads(t, suite.users, userID)
	assertUserPermissions(t, suite.users, userID)
	assertUserMutations(t, suite.users, userID)
	assertUserFailures(t, suite, userID)
}

func assertUserReads(t *testing.T, repository *postgres.UserRepo, userID uuid.UUID) {
	user, err := repository.GetByID(context.Background(), userID)
	requireNoError(t, err)
	if user.Username != "admin" || user.RoleID.String() != superadminRoleID {
		t.Fatalf("unexpected user scan: %#v", user)
	}
	byEmail, err := repository.GetByEmail(context.Background(), "admin@example.test")
	requireNoError(t, err)
	if byEmail.ID != userID {
		t.Fatalf("GetByEmail() id = %s, want %s", byEmail.ID, userID)
	}
	role, err := repository.GetRoleByID(context.Background(), uuid.MustParse(superadminRoleID))
	requireNoError(t, err)
	if role.Rank != 0 {
		t.Fatalf("superadmin rank = %d, want 0", role.Rank)
	}
}

func assertUserPermissions(t *testing.T, repository *postgres.UserRepo, userID uuid.UUID) {
	allowed, err := repository.HasPermission(context.Background(), userID, "applications", "deploy")
	requireNoError(t, err)
	if !allowed {
		t.Fatal("superadmin deploy permission = false, want true")
	}
	allowed, err = repository.HasPermission(context.Background(), userID, "missing", "read")
	requireNoError(t, err)
	if allowed {
		t.Fatal("missing permission = true, want false")
	}
	admins, err := repository.CountAdmins(context.Background())
	requireNoError(t, err)
	if admins != 1 {
		t.Fatalf("CountAdmins() = %d, want 1", admins)
	}
}

func assertUserMutations(t *testing.T, repository *postgres.UserRepo, userID uuid.UUID) {
	requireNoError(t, repository.UpdateRefreshToken(context.Background(), userID, "refresh-token"))
	requireNoError(t, repository.UpdateUserRole(context.Background(), userID, uuid.MustParse(viewerRoleID)))
	admins, err := repository.CountAdmins(context.Background())
	requireNoError(t, err)
	if admins != 0 {
		t.Fatalf("CountAdmins() after demotion = %d, want 0", admins)
	}
}

func assertUserFailures(t *testing.T, suite *contractSuite, userID uuid.UUID) {
	missingID := uuid.New()
	assertNotFound(t, suite.users.UpdateRefreshToken(context.Background(), missingID, "token"))
	assertNotFound(t, suite.users.UpdateUserRole(context.Background(), missingID, uuid.MustParse(viewerRoleID)))
	_, err := suite.users.GetByID(context.Background(), missingID)
	assertNotFound(t, err)
	_, err = suite.users.GetByEmail(context.Background(), "missing@example.test")
	assertNotFound(t, err)
	_, err = suite.users.GetRoleByID(context.Background(), missingID)
	assertNotFound(t, err)
	_, err = suite.pool.Exec(context.Background(), `
		INSERT INTO users (username, email, password_hash, role_id)
		VALUES ('duplicate', 'admin@example.test', 'hash', $1)`, viewerRoleID)
	if err == nil {
		t.Fatalf("duplicate email for user %s unexpectedly succeeded", userID)
	}
}

func testDomainRepository(t *testing.T, suite *contractSuite) {
	userID := insertUser(t, suite.pool, superadminRoleID, "domain-owner")
	record := newDomain(userID, "app.example.test")
	requireNoError(t, suite.domains.Create(context.Background(), record))
	assertDomainReads(t, suite.domains, record, userID)
	assertDomainMutations(t, suite.domains, record)
	assertDomainFailures(t, suite.domains, userID)
}

func assertDomainReads(t *testing.T, repository *postgres.DomainRepository, record *domain.Domain, userID uuid.UUID) {
	listed, err := repository.ListByUser(context.Background(), userID)
	requireNoError(t, err)
	if len(listed) != 1 || listed[0].DomainName != record.DomainName {
		t.Fatalf("ListByUser() = %#v", listed)
	}
	fetched, err := repository.GetByID(context.Background(), record.ID, userID)
	requireNoError(t, err)
	if fetched.DocumentRoot != record.DocumentRoot || fetched.AppID != uuid.Nil {
		t.Fatalf("GetByID() = %#v", fetched)
	}
	byApp, err := repository.GetByAppID(context.Background(), uuid.New())
	requireNoError(t, err)
	if len(byApp) != 0 {
		t.Fatalf("GetByAppID() count = %d, want 0", len(byApp))
	}
}

func assertDomainMutations(t *testing.T, repository *postgres.DomainRepository, record *domain.Domain) {
	requireNoError(t, repository.UpdateStatus(context.Background(), record.DomainName, "active"))
	requireNoError(t, repository.MarkRenewalStatus(context.Background(), record.DomainName, "active"))
	active, err := repository.GetDomainsWithActiveSSL(context.Background())
	requireNoError(t, err)
	due, dueErr := repository.FindDueForRenewal(context.Background())
	requireNoError(t, dueErr)
	if len(active) != 1 || len(due) != 1 {
		t.Fatalf("active/due counts = %d/%d, want 1/1", len(active), len(due))
	}
	requireNoError(t, repository.Delete(context.Background(), record.DomainName))
	assertNotFound(t, repository.Delete(context.Background(), record.DomainName))
}

func assertDomainFailures(t *testing.T, repository *postgres.DomainRepository, userID uuid.UUID) {
	missing, err := repository.GetByID(context.Background(), uuid.New(), userID)
	if missing != nil {
		t.Fatalf("missing domain = %#v, want nil", missing)
	}
	assertNotFound(t, err)
	first := newDomain(userID, "duplicate.example.test")
	second := newDomain(userID, "duplicate.example.test")
	requireNoError(t, repository.Create(context.Background(), first))
	if err := repository.Create(context.Background(), second); err == nil {
		t.Fatal("duplicate domain unexpectedly succeeded")
	}
}

func testApplicationRepository(t *testing.T, suite *contractSuite) {
	ownerID := insertUser(t, suite.pool, superadminRoleID, "app-owner")
	domainRecord := newDomain(ownerID, "application.example.test")
	requireNoError(t, suite.domains.Create(context.Background(), domainRecord))
	app := newApplication(ownerID, domainRecord.ID)
	requireNoError(t, suite.applications.Create(context.Background(), app))
	assertApplicationReads(t, suite.applications, app, ownerID)
	assertApplicationMutations(t, suite.applications, app)
	assertApplicationFailures(t, suite, app)
}

func assertApplicationReads(t *testing.T, repository domain.ApplicationRepository, app *domain.Application, ownerID uuid.UUID) {
	fetched, err := repository.GetByID(context.Background(), app.ID, ownerID)
	requireNoError(t, err)
	if fetched.Name != app.Name || fetched.DomainName != "application.example.test" {
		t.Fatalf("GetByID() = %#v", fetched)
	}
	metadata, err := repository.GetByIDWithMetadata(context.Background(), app.ID)
	requireNoError(t, err)
	if metadata.OwnerID != ownerID || metadata.OwnerRank != 0 {
		t.Fatalf("GetByIDWithMetadata() = %#v", metadata)
	}
}

func assertApplicationMutations(t *testing.T, repository domain.ApplicationRepository, app *domain.Application) {
	requireNoError(t, repository.UpdateEnvVars(context.Background(), app.ID, map[string]string{"MODE": "test"}))
	requireNoError(t, repository.UpdateStatus(context.Background(), app.ID, "running"))
	active, err := repository.ListAllActive(context.Background())
	requireNoError(t, err)
	if len(active) != 1 || active[0].EnvVars["MODE"] != "test" {
		t.Fatalf("ListAllActive() = %#v", active)
	}
	requireNoError(t, repository.Delete(context.Background(), app.ID))
	assertNotFound(t, repository.Delete(context.Background(), app.ID))
}

func assertApplicationFailures(t *testing.T, suite *contractSuite, app *domain.Application) {
	missingID := uuid.New()
	_, err := suite.applications.GetByID(context.Background(), missingID, app.OwnerID)
	assertNotFound(t, err)
	_, err = suite.applications.GetByIDWithMetadata(context.Background(), missingID)
	assertNotFound(t, err)
	assertNotFound(t, suite.applications.UpdateStatus(context.Background(), missingID, "failed"))
	bad := newApplication(app.OwnerID, uuid.New())
	if err := suite.applications.Create(context.Background(), bad); err == nil {
		t.Fatal("application with missing domain unexpectedly succeeded")
	}
}

func testAuditRepository(t *testing.T, suite *contractSuite) {
	resourceID := uuid.New()
	alert := &domain.SystemAlert{
		Severity: "warning", Category: "contract", ResourceID: resourceID,
		Message: "contract alert", Metadata: map[string]any{"trace_id": "trace-contract"},
	}
	requireNoError(t, suite.audits.CreateAlert(context.Background(), alert))
	assertAuditFilters(t, suite.audits, alert, resourceID)
	requireNoError(t, suite.audits.ResolveAlert(context.Background(), alert.ID, uuid.New()))
	if err := suite.audits.ResolveAlert(context.Background(), alert.ID, uuid.New()); err == nil {
		t.Fatal("resolving an already resolved alert unexpectedly succeeded")
	}
}

func assertAuditFilters(t *testing.T, repository domain.AuditRepository, alert *domain.SystemAlert, resourceID uuid.UUID) {
	unresolved := false
	filters := []domain.AlertFilter{
		{ResourceID: resourceID, Limit: 10},
		{Severity: "warning", Limit: 10},
		{IsResolved: &unresolved, Limit: 10},
		{TraceID: "trace-contract", Limit: 10},
	}
	for _, filter := range filters {
		alerts, count, err := repository.GetFilteredAlerts(context.Background(), filter)
		requireNoError(t, err)
		if count != 1 || len(alerts) != 1 || alerts[0].ID != alert.ID {
			t.Fatalf("GetFilteredAlerts(%#v) = count %d, alerts %#v", filter, count, alerts)
		}
	}
}

func testDeploymentRepository(t *testing.T, suite *contractSuite) {
	ownerID := insertUser(t, suite.pool, superadminRoleID, "deploy-owner")
	domainRecord := newDomain(ownerID, "deploy.example.test")
	requireNoError(t, suite.domains.Create(context.Background(), domainRecord))
	app := newApplication(ownerID, domainRecord.ID)
	requireNoError(t, suite.applications.Create(context.Background(), app))
	assertDeploymentQueue(t, suite, app)
	assertConcurrentClaims(t, suite, app)
	assertDeploymentFailures(t, suite.deployments, app.ID)
}

func assertDeploymentQueue(t *testing.T, suite *contractSuite, app *domain.Application) {
	deployment := newDeployment(app.ID)
	requireNoError(t, suite.deployments.Save(context.Background(), deployment))
	requireNoError(t, suite.deployments.AppendLog(context.Background(), deployment.ID, "queued"))
	claimed, err := suite.deployments.ClaimNextPending(context.Background())
	requireNoError(t, err)
	if claimed == nil || claimed.ID != deployment.ID || claimed.Status != domain.StatusRunning {
		t.Fatalf("ClaimNextPending() = %#v", claimed)
	}
	requireNoError(t, suite.deployments.UpdateStatus(context.Background(), deployment.ID, domain.StatusSuccess))
	empty, err := suite.deployments.ClaimNextPending(context.Background())
	requireNoError(t, err)
	if empty != nil {
		t.Fatalf("empty queue claim = %#v", empty)
	}
}

func assertConcurrentClaims(t *testing.T, suite *contractSuite, app *domain.Application) {
	first := newDeployment(app.ID)
	second := newDeployment(app.ID)
	requireNoError(t, suite.deployments.Save(context.Background(), first))
	requireNoError(t, suite.deployments.Save(context.Background(), second))
	claims := make(chan string, 2)
	errorsChannel := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go claimDeployment(&group, suite.deployments, claims, errorsChannel)
	}
	group.Wait()
	close(claims)
	close(errorsChannel)
	for err := range errorsChannel {
		requireNoError(t, err)
	}
	distinct := map[string]bool{}
	for id := range claims {
		distinct[id] = true
	}
	if len(distinct) != 2 {
		t.Fatalf("concurrent claim ids = %#v, want two distinct ids", distinct)
	}
}

func claimDeployment(group *sync.WaitGroup, repository *postgres.PostgresDeploymentRepository, claims chan<- string, errs chan<- error) {
	defer group.Done()
	claimed, err := repository.ClaimNextPending(context.Background())
	if err != nil {
		errs <- err
		return
	}
	if claimed == nil {
		errs <- errors.New("concurrent claim returned nil")
		return
	}
	claims <- claimed.ID
}

func assertDeploymentFailures(t *testing.T, repository *postgres.PostgresDeploymentRepository, appID uuid.UUID) {
	missingID := uuid.NewString()
	assertNotFound(t, repository.UpdateStatus(context.Background(), missingID, domain.StatusFailed))
	if err := repository.AppendLog(context.Background(), missingID, "missing"); err == nil {
		t.Fatal("deployment log with missing foreign key unexpectedly succeeded")
	}
	deployment := newDeployment(appID)
	deployment.Status = domain.Status("invalid")
	if err := repository.Save(context.Background(), deployment); err == nil {
		t.Fatal("deployment with invalid status unexpectedly succeeded")
	}
}

func testProfileRepository(t *testing.T, suite *contractSuite) {
	profile, err := suite.profiles.GetActiveProfile(context.Background())
	requireNoError(t, err)
	stale := *profile
	profile.BackupRetentionDays = 45
	requireNoError(t, suite.profiles.UpdateProfile(context.Background(), profile))
	if profile.Version != 2 {
		t.Fatalf("profile version = %d, want 2", profile.Version)
	}
	if err := suite.profiles.UpdateProfile(context.Background(), &stale); !errors.Is(err, karidb.ErrConcurrencyConflict) {
		t.Fatalf("stale profile update error = %v, want ErrConcurrencyConflict", err)
	}
	_, err = suite.pool.Exec(context.Background(), "DELETE FROM system_profiles")
	requireNoError(t, err)
	_, err = suite.profiles.GetActiveProfile(context.Background())
	if !errors.Is(err, karidb.ErrProfileNotFound) {
		t.Fatalf("missing profile error = %v, want ErrProfileNotFound", err)
	}
}

func insertUser(t *testing.T, pool *pgxpool.Pool, roleID string, username string) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	email := fmt.Sprintf("%s@example.test", username)
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, password_hash, role_id)
		VALUES ($1, $2, $3, 'hash', $4)`, userID, username, email, roleID)
	requireNoError(t, err)
	return userID
}

func newDomain(userID uuid.UUID, name string) *domain.Domain {
	return &domain.Domain{
		UserID: userID, DomainName: name, DocumentRoot: "/srv/kari/" + name,
		SSLStatus: "none", Status: "provisioning", ExpiresAt: time.Now().UTC().Add(20 * 24 * time.Hour),
	}
}

func newApplication(ownerID uuid.UUID, domainID uuid.UUID) *domain.Application {
	identifier := uuid.NewString()
	return &domain.Application{
		Name: "contract-app", DomainID: domainID, AppType: "nodejs", OwnerID: ownerID,
		AppUser: "kari-" + identifier[:8], RepoURL: "https://example.test/repository.git",
		Branch: "main", BuildCommand: "npm run build", StartCommand: "node server.js",
		EnvVars: map[string]string{"MODE": "contract"}, Port: 3000, Status: "stopped",
		WebhookSecret: "contract-secret",
	}
}

func newDeployment(appID uuid.UUID) *domain.Deployment {
	return &domain.Deployment{
		ID: uuid.NewString(), AppID: appID.String(), DomainName: "deploy.example.test",
		RepoURL: "https://example.test/repository.git", Branch: "main",
		BuildCommand: "npm run build", TargetPort: 3000,
		EnvVars: map[string]string{"MODE": "contract"}, Status: domain.StatusPending,
	}
}

func assertCount(t *testing.T, pool *pgxpool.Pool, query string, want int) {
	t.Helper()
	var count int
	requireNoError(t, pool.QueryRow(context.Background(), query).Scan(&count))
	if count != want {
		t.Fatalf("count for %q = %d, want %d", query, count, want)
	}
}

func assertNotFound(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want domain.ErrNotFound", err)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
