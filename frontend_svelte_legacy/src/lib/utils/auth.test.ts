import { describe, it, expect } from 'vitest';
import { canPerform, canPerformAny, canPerformAll, type KariUser } from './auth';

describe('auth utilities', () => {
	const adminUser: KariUser = {
		id: '1',
		email: 'admin@kari.io',
		rank: 'admin',
		permissions: []
	};

	const tenantUser: KariUser = {
		id: '2',
		email: 'tenant@kari.io',
		rank: 'tenant',
		permissions: ['apps:read', 'apps:write', 'domains:*']
	};

	const viewerUser: KariUser = {
		id: '3',
		email: 'viewer@kari.io',
		rank: 'viewer',
		permissions: ['apps:read', '*:*']
	};

	const restrictedUser: KariUser = {
		id: '4',
		email: 'restricted@kari.io',
		rank: 'viewer',
		permissions: ['apps:read']
	};

	describe('canPerform', () => {
		it('should return false if user is null or undefined', () => {
			expect(canPerform(null, 'apps:read')).toBe(false);
			expect(canPerform(undefined, 'apps:read')).toBe(false);
		});

		it('should return true for admin rank regardless of permissions', () => {
			expect(canPerform(adminUser, 'any:permission')).toBe(true);
			expect(canPerform(adminUser, 'system:root')).toBe(true);
		});

		it('should return true for exact permission match', () => {
			expect(canPerform(tenantUser, 'apps:read')).toBe(true);
			expect(canPerform(tenantUser, 'apps:write')).toBe(true);
		});

		it('should return true for resource wildcard', () => {
			expect(canPerform(tenantUser, 'domains:create')).toBe(true);
			expect(canPerform(tenantUser, 'domains:delete')).toBe(true);
		});

		it('should return true for global wildcard', () => {
			expect(canPerform(viewerUser, 'anything:at-all')).toBe(true);
		});

		it('should return false if permission is not granted', () => {
			expect(canPerform(restrictedUser, 'apps:write')).toBe(false);
			expect(canPerform(restrictedUser, 'domains:read')).toBe(false);
		});

		it('should return false for partial matches that are not wildcards', () => {
			expect(canPerform(restrictedUser, 'apps')).toBe(false);
		});
	});

	describe('canPerformAny', () => {
		it('should return false if user is null or undefined', () => {
			expect(canPerformAny(null, ['apps:read'])).toBe(false);
			expect(canPerformAny(undefined, ['apps:read'])).toBe(false);
		});

		it('should return false for empty permissions list', () => {
			expect(canPerformAny(tenantUser, [])).toBe(false);
		});

		it('should return true if at least one permission matches', () => {
			expect(canPerformAny(restrictedUser, ['apps:write', 'apps:read'])).toBe(true);
		});

		it('should return false if no permissions match', () => {
			expect(canPerformAny(restrictedUser, ['apps:write', 'domains:read'])).toBe(false);
		});
	});

	describe('canPerformAll', () => {
		it('should return false if user is null or undefined', () => {
			expect(canPerformAll(null, ['apps:read'])).toBe(false);
			expect(canPerformAll(undefined, ['apps:read'])).toBe(false);
		});

		it('should return true for empty permissions list', () => {
			expect(canPerformAll(tenantUser, [])).toBe(true);
		});

		it('should return true if all permissions match', () => {
			expect(canPerformAll(tenantUser, ['apps:read', 'domains:any'])).toBe(true);
		});

		it('should return false if some permissions do not match', () => {
			expect(canPerformAll(tenantUser, ['apps:read', 'system:write'])).toBe(false);
		});
	});
});
