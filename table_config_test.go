package wirelog

import (
	"strings"
	"testing"
)

// TestDefaultTableUnchanged pins the backward-compatible default so existing
// callers keep writing to provider_api_logs.
func TestDefaultTableUnchanged(t *testing.T) {
	if defaultOptions().table != "provider_api_logs" {
		t.Fatalf("default table = %q, want provider_api_logs", defaultOptions().table)
	}
}

// TestWithTableValidation accepts safe identifiers and ignores anything that
// could break out of the interpolated table name, keeping the default instead.
func TestWithTableValidation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"valid inbound", "inbound_api_logs", "inbound_api_logs"},
		{"valid leading underscore", "_scratch", "_scratch"},
		{"valid mixed case and digits", "Audit1_Log", "Audit1_Log"},
		{"empty keeps default", "", defaultTable},
		{"leading digit rejected", "1logs", defaultTable},
		{"hyphen rejected", "api-logs", defaultTable},
		{"injection rejected", "logs; drop table x", defaultTable},
		{"whitespace rejected", "api logs", defaultTable},
		{"too long rejected", strings.Repeat("a", 46), defaultTable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := defaultOptions()
			WithTable(tt.input)(&o)
			if o.table != tt.want {
				t.Errorf("WithTable(%q) → table %q, want %q", tt.input, o.table, tt.want)
			}
		})
	}
}

// TestSchemaDDLTargetsTable renders the schema for a custom table with per-table
// index names, so two wirelog tables never collide on index names in one DB.
func TestSchemaDDLTargetsTable(t *testing.T) {
	ddl := schemaDDL("inbound_api_logs")

	wants := []string{
		"create table if not exists inbound_api_logs",
		"alter table inbound_api_logs add column if not exists remote_ip text",
		"idx_inbound_api_logs_provider_time on inbound_api_logs (provider, created_at desc)",
		"idx_inbound_api_logs_resp_body_gin on inbound_api_logs using gin (response_body jsonb_path_ops)",
	}
	for _, want := range wants {
		if !strings.Contains(ddl, want) {
			t.Errorf("schemaDDL(inbound_api_logs) missing %q", want)
		}
	}
	// index names are derived from the table, so the default prefix must not leak
	if strings.Contains(ddl, "on provider_api_logs") || strings.Contains(ddl, "idx_provider_api_logs_") {
		t.Error("custom-table DDL leaked the default table name")
	}
}

// TestBuildInsertTargetsTable confirms the INSERT is rendered against the given
// table.
func TestBuildInsertTargetsTable(t *testing.T) {
	recs := []record{{provider: "zobo-be", outcome: outcomeSuccess}}
	sql, _ := buildInsert(recs, "inbound_api_logs")
	if !strings.HasPrefix(sql, "insert into inbound_api_logs (") {
		t.Errorf("INSERT target wrong: %.40q", sql)
	}
	if strings.Contains(sql, "provider_api_logs") {
		t.Error("custom-table INSERT leaked the default table name")
	}
}
