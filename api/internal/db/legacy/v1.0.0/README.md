# Published v1.0.0 SQL archive

These files were distributed with Kari v1.0.0 and are preserved byte-for-byte as
release evidence. They are not an executable migration chain and must never be
mounted into PostgreSQL's `/docker-entrypoint-initdb.d` directory.

| File | SHA-256 |
| --- | --- |
| `0002_deployments.sql` | `0b07ba7d87f040aa1f7b7823fd854b1abfd62878561a8c8a383fb6eadc696643` |
| `001_init.sql` | `9a7cf5d73975576915c7ac13060107284a08adf0fcf125eeedc7d13b7b7554cb` |
| `001_initial_schema.sql` | `a9f193ea16bcc8a9f4e7b5abb321c00139fb9a14811c079ffd5b4afa9b44d58e` |
| `002_gitops_observability.sql` | `021c7dc5953b78a7e2a8a8dc2eb77d2d434b49ba63e6bf9f50d4b8f0201791bf` |
| `003_dynamic_rbac.sql` | `58ee2882120bf62ea7c30cbfcf66f81bcac39f4cac225c882da2258c921c88d5` |
| `ssl_schema.sql` | `c3f39d63863c4d62c7439c341fe1d269d65d1c21079ffd3756a064b08d5bfec8` |

The owner authorized a clean migration baseline on 2026-08-25. Existing v1.0.0
database installations are not supported by the baseline migration authority.
