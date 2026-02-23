export type DeploymentStatus = 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED';

export interface Deployment {
	id: string;
	domain_name: string;
	status: DeploymentStatus;
	branch: string;
	created_at: string;
	// Optional fields that might be returned but not currently used in the table
	app_id?: string;
	repo_url?: string;
	build_command?: string;
	target_port?: number;
}
