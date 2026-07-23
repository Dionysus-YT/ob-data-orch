ALTER TABLE data_sources ADD COLUMN cluster_name TEXT NOT NULL DEFAULT '' CHECK (length(cluster_name) <= 255);
ALTER TABLE data_sources ADD COLUMN tenant_name TEXT NOT NULL DEFAULT '' CHECK (length(tenant_name) <= 255);
