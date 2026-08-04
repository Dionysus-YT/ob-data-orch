ALTER TABLE agents ADD COLUMN facts_revision INTEGER NOT NULL DEFAULT 0 CHECK (facts_revision >= 0);
