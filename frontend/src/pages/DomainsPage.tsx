import { useEffect, useState } from 'react'
import { apiDelete, apiGet, apiPost } from '../lib/api'

type SslStatus = 'none' | 'pending' | 'active' | 'failed'

interface Domain {
  id: string
  domain_name: string
  ssl_status: SslStatus
  created_at: string
}

const isValidDomain = (d: string) => /^[a-z0-9]+([-.]{1}[a-z0-9]+)*\.[a-z]{2,5}$/i.test(d)

export function DomainsPage() {
  const [domains, setDomains] = useState<Domain[]>([])
  const [newDomainName, setNewDomainName] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [actionStates, setActionStates] = useState<Record<string, 'provisioning' | 'deleting'>>({})
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void apiGet<Domain[]>('/api/v1/domains')
      .then(setDomains)
      .catch((err) => {
        console.error('Failed to load domains', err)
        setError('Failed to load domains.')
      })
  }, [])

  async function handleAddDomain(event: React.FormEvent) {
    event.preventDefault()
    if (!isValidDomain(newDomainName) || isSubmitting) {
      setError('Please enter a valid FQDN (e.g., app.kari.io)')
      return
    }
    setIsSubmitting(true)
    setError(null)
    try {
      const newDomain = await apiPost<{ domain_name: string }, Domain>('/api/v1/domains', {
        domain_name: newDomainName.toLowerCase(),
      })
      setDomains((prev) => [newDomain, ...prev])
      setNewDomainName('')
    } catch (e: unknown) {
      console.error('Failed to add domain', e)
      setError(e instanceof Error ? e.message : 'Brain rejected the domain registration.')
    } finally {
      setIsSubmitting(false)
    }
  }

  async function handleProvisionSsl(domainId: string) {
    if (actionStates[domainId]) return
    setActionStates((prev) => ({ ...prev, [domainId]: 'provisioning' }))
    setError(null)
    try {
      await apiPost<Record<string, never>, void>(`/api/v1/domains/${domainId}/ssl`, {})
      setDomains((prev) =>
        prev.map((d) => (d.id === domainId ? { ...d, ssl_status: 'active' as SslStatus } : d)),
      )
    } catch (e: unknown) {
      console.error('Failed to provision SSL', e)
      setError(e instanceof Error ? e.message : 'SSL provisioning failed.')
      setDomains((prev) =>
        prev.map((d) => (d.id === domainId ? { ...d, ssl_status: 'failed' as SslStatus } : d)),
      )
    } finally {
      setActionStates((prev) => {
        const next = { ...prev }
        delete next[domainId]
        return next
      })
    }
  }

  async function handleDelete(domainId: string) {
    if (!window.confirm('This will purge the domain and all associated SSL certificates. Proceed?'))
      return
    setActionStates((prev) => ({ ...prev, [domainId]: 'deleting' }))
    try {
      await apiDelete(`/api/v1/domains/${domainId}`)
      setDomains((prev) => prev.filter((d) => d.id !== domainId))
    } catch (e: unknown) {
      console.error('Failed to delete domain', e)
      setError('Deletion failed.')
    } finally {
      setActionStates((prev) => {
        const next = { ...prev }
        delete next[domainId]
        return next
      })
    }
  }

  function sslIcon(domain: Domain) {
    if (domain.ssl_status === 'active') return '🔒'
    if (actionStates[domain.id] === 'provisioning') return '⏳'
    if (domain.ssl_status === 'failed') return '⚠️'
    return '🔓'
  }

  return (
    <div className="page stack">
      <div className="page-title">
        <h1>Domains</h1>
        <p>Manage public domain names and SSL certificates.</p>
      </div>

      {error ? <div className="alert error">{error}</div> : null}

      <form className="card" onSubmit={handleAddDomain}>
        <div className="form-grid">
          <label className="field">
            <span>Domain Name</span>
            <input
              type="text"
              placeholder="app.production.io"
              value={newDomainName}
              onChange={(e) => setNewDomainName(e.target.value)}
              required
            />
          </label>
        </div>
        <div className="form-actions">
          <button type="submit" disabled={isSubmitting || !newDomainName}>
            {isSubmitting ? 'Adding…' : 'Add Domain'}
          </button>
        </div>
      </form>

      <section className="card">
        <div className="section-header">
          <h2>Registered Domains</h2>
        </div>
        {domains.length === 0 ? (
          <p className="muted">No domains registered. Add one above to get started.</p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Domain</th>
                  <th>SSL Status</th>
                  <th>Added</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {domains.map((domain) => (
                  <tr key={domain.id}>
                    <td>
                      <code>{domain.domain_name}</code>
                    </td>
                    <td>
                      <span title={domain.ssl_status}>
                        {sslIcon(domain)} {domain.ssl_status}
                      </span>
                    </td>
                    <td>{new Date(domain.created_at).toLocaleDateString()}</td>
                    <td className="right">
                      <div className="inline-actions">
                        {domain.ssl_status !== 'active' ? (
                          <button
                            type="button"
                            disabled={!!actionStates[domain.id]}
                            onClick={() => void handleProvisionSsl(domain.id)}
                          >
                            {actionStates[domain.id] === 'provisioning'
                              ? 'Provisioning…'
                              : 'Enable SSL'}
                          </button>
                        ) : null}
                        <button
                          type="button"
                          disabled={!!actionStates[domain.id]}
                          onClick={() => void handleDelete(domain.id)}
                        >
                          {actionStates[domain.id] === 'deleting' ? 'Deleting…' : 'Delete'}
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}
