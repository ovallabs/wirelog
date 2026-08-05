// migrate.go — the archive's floor plan: the embedded DDL for
// provider_api_logs, applied only when auto-migrate is enabled.

package wirelog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// defaultTable is the table wirelog reads and writes unless WithTable overrides
// it, preserving the original schema name for existing callers.
const defaultTable = "provider_api_logs"

// execer is the slice of pgxpool.Pool that migration needs; kept small so
// tests can fake it without a database.
type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// schemaDDL renders the FRD schema for table, deriving per-table index names so
// two wirelog tables can coexist in one database without colliding. table is a
// validated identifier (see WithTable), so interpolating it here is safe.
func schemaDDL(table string) string {
	return fmt.Sprintf(`create table if not exists %[1]s (
    id               bigint generated always as identity primary key,
    created_at       timestamptz not null default now(),
    provider         text        not null,
    consumer         text        not null default '',
    operation        text        not null default '',
    endpoint         text        not null default '',
    path             text        not null default '',
    method           text        not null default '',
    remote_ip        text,
    status_code      int,
    outcome          text        not null,
    latency_ms       bigint      not null default 0,
    request_size     bigint      not null default 0,
    response_size    bigint      not null default 0,
    internal_ref     text,
    idempotency_key  text,
    request_headers  jsonb,
    request_body     jsonb,
    response_headers jsonb,
    response_body    jsonb,
    error            text,
    tags             jsonb
);
alter table %[1]s add column if not exists remote_ip text;
create index if not exists idx_%[1]s_provider_time on %[1]s (provider, created_at desc);
create index if not exists idx_%[1]s_consumer_time on %[1]s (consumer, created_at desc);
create index if not exists idx_%[1]s_internal_ref  on %[1]s (internal_ref) where internal_ref is not null;
create index if not exists idx_%[1]s_idem_key      on %[1]s (idempotency_key) where idempotency_key is not null;
create index if not exists idx_%[1]s_failures      on %[1]s (created_at desc) where outcome <> 'success';
create index if not exists idx_%[1]s_req_body_gin  on %[1]s using gin (request_body  jsonb_path_ops);
create index if not exists idx_%[1]s_resp_body_gin on %[1]s using gin (response_body jsonb_path_ops);
`, table)
}

// migrate applies the schema DDL for table; idempotent via IF NOT EXISTS throughout.
func migrate(ctx context.Context, db execer, table string) error {
	_, err := db.Exec(ctx, schemaDDL(table))
	return err
}
