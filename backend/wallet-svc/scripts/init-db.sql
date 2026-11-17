-- Roles for the wallet database.
-- wallet_migrate: DDL during bun/migrate (cmd/migrate only).
-- wallet_app: DML only. SELECT+INSERT on ledger_entries (no UPDATE/DELETE).
-- wallet_readonly: reporting.

DO
$do$
BEGIN
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'wallet_migrate') THEN
      CREATE USER wallet_migrate WITH PASSWORD 'wallet_migrate_password';
   END IF;
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'wallet_app') THEN
      CREATE USER wallet_app WITH PASSWORD 'wallet_app_password';
   END IF;
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'wallet_readonly') THEN
      CREATE USER wallet_readonly WITH PASSWORD 'wallet_readonly_password';
   END IF;
END
$do$;

GRANT CONNECT ON DATABASE wallet TO wallet_migrate, wallet_app, wallet_readonly;
GRANT ALL ON SCHEMA public TO wallet_migrate;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO wallet_migrate;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO wallet_migrate;

GRANT USAGE ON SCHEMA public TO wallet_app, wallet_readonly;
