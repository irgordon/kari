import { useEffect, useState } from 'react'
import { apiGet, apiPut } from '../lib/api'

interface Role {
  id: string
  name: string
  description: string
  is_system: boolean
}

interface Permission {
  id: string
  resource: string
  action: string
  description: string
}

type PermissionMatrix = Record<string, Permission[]>
type RolePermissionMap = Record<string, string[]>

export function RolesPage() {
  const [roles, setRoles] = useState<Role[]>([])
  const [permissionMatrix, setPermissionMatrix] = useState<PermissionMatrix>({})
  const [draftMappings, setDraftMappings] = useState<RolePermissionMap>({})
  const [selectedRoleId, setSelectedRoleId] = useState<string | null>(null)
  const [isSaving, setIsSaving] = useState(false)
  const [saveMessage, setSaveMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void Promise.all([
      apiGet<Role[]>('/api/v1/roles'),
      apiGet<Permission[]>('/api/v1/permissions'),
      apiGet<RolePermissionMap>('/api/v1/roles/mappings'),
    ])
      .then(([rolesData, permsData, mappingsData]) => {
        setRoles(rolesData)
        setDraftMappings(mappingsData)

        const matrix: PermissionMatrix = {}
        for (const perm of permsData) {
          if (!matrix[perm.resource]) {
            matrix[perm.resource] = []
          }
          matrix[perm.resource].push(perm)
        }
        setPermissionMatrix(matrix)

        if (rolesData.length > 0) {
          setSelectedRoleId(rolesData[0].id)
        }
      })
      .catch((err) => {
        console.error('Failed to load RBAC data', err)
        setError('Failed to load roles and permissions.')
      })
  }, [])

  const selectedRole = roles.find((r) => r.id === selectedRoleId)
  const currentRolePerms = new Set(draftMappings[selectedRoleId ?? ''] ?? [])

  function selectRole(id: string) {
    setSelectedRoleId(id)
    setSaveMessage(null)
  }

  function togglePermission(permissionId: string) {
    if (!selectedRoleId || selectedRole?.is_system) return
    setDraftMappings((prev) => {
      const current = new Set(prev[selectedRoleId] ?? [])
      if (current.has(permissionId)) {
        current.delete(permissionId)
      } else {
        current.add(permissionId)
      }
      return { ...prev, [selectedRoleId]: Array.from(current) }
    })
  }

  async function saveRolePermissions() {
    if (!selectedRoleId || isSaving || selectedRole?.is_system) return
    setIsSaving(true)
    setSaveMessage(null)
    try {
      await apiPut<{ permission_ids: string[] }, void>(
        `/api/v1/roles/${selectedRoleId}/permissions`,
        { permission_ids: draftMappings[selectedRoleId] ?? [] },
      )
      setSaveMessage({ type: 'success', text: 'Role permissions successfully updated.' })
      setTimeout(() => setSaveMessage(null), 3000)
    } catch (e: unknown) {
      console.error('Failed to save role permissions', e)
      setSaveMessage({
        type: 'error',
        text: e instanceof Error ? e.message : 'Failed to update permissions.',
      })
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <div className="page stack">
      <div className="page-title">
        <h1>Access Control</h1>
        <p>Define granular RBAC policies for your team. System roles cannot be modified.</p>
      </div>

      {error ? <div className="alert error">{error}</div> : null}

      <div className="roles-layout">
        {/* Role list */}
        <section className="card roles-sidebar">
          <div className="section-header">
            <h2>Roles</h2>
          </div>
          <ul className="list">
            {roles.map((role) => (
              <li key={role.id}>
                <button
                  type="button"
                  className={`role-item${selectedRoleId === role.id ? ' selected' : ''}`}
                  onClick={() => selectRole(role.id)}
                >
                  <div className="role-item-row">
                    <span className="role-item-name">{role.name}</span>
                    {role.is_system ? <span className="badge">System</span> : null}
                  </div>
                  <p className="muted small">{role.description}</p>
                </button>
              </li>
            ))}
          </ul>
        </section>

        {/* Permission matrix */}
        <section className="card roles-detail stack">
          {selectedRole ? (
            <>
              <div className="section-header">
                <div>
                  <h2>{selectedRole.name} Permissions</h2>
                  {selectedRole.is_system ? (
                    <p className="muted small">
                      This is a protected system role and cannot be modified.
                    </p>
                  ) : (
                    <p className="muted small">Toggle permissions below to customize access.</p>
                  )}
                </div>
              </div>

              <div className="stack">
                {Object.entries(permissionMatrix).map(([resource, perms]) => (
                  <div key={resource} className="card">
                    <div className="section-header">
                      <h3 className="capitalize">{resource.replace(/_/g, ' ')}</h3>
                    </div>
                    <ul className="list">
                      {perms.map((perm) => (
                        <li key={perm.id} className="perm-row">
                          <div>
                            <p className="capitalize">{perm.action}</p>
                            <p className="muted small">{perm.description}</p>
                          </div>
                          <button
                            type="button"
                            role="switch"
                            aria-checked={currentRolePerms.has(perm.id)}
                            disabled={selectedRole.is_system}
                            onClick={() => togglePermission(perm.id)}
                            className={`toggle${currentRolePerms.has(perm.id) ? ' on' : ''}`}
                          >
                            <span className="toggle-thumb" />
                          </button>
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>

              {!selectedRole.is_system ? (
                <div className="form-actions">
                  {saveMessage ? (
                    <span
                      className={saveMessage.type === 'error' ? 'alert error' : 'alert success'}
                    >
                      {saveMessage.text}
                    </span>
                  ) : null}
                  <button
                    type="button"
                    disabled={isSaving}
                    onClick={() => void saveRolePermissions()}
                  >
                    {isSaving ? 'Saving…' : 'Save Policies'}
                  </button>
                </div>
              ) : null}
            </>
          ) : (
            <p className="muted">Select a role from the left pane to configure its permissions.</p>
          )}
        </section>
      </div>
    </div>
  )
}
