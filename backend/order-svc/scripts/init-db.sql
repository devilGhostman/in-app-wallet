-- Roles for the orders database (POSTGRES_DB=orders creates the DB).
-- order_migrate: DDL during bun/migrate.
-- order_app: DML for the API.

DO
$do$
BEGIN
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'order_migrate') THEN
      CREATE USER order_migrate WITH PASSWORD 'order_migrate_password';
   END IF;
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'order_app') THEN
      CREATE USER order_app WITH PASSWORD 'order_app_password';
   END IF;
END
$do$;

GRANT CONNECT ON DATABASE orders TO order_migrate, order_app;
GRANT ALL ON SCHEMA public TO order_migrate;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO order_migrate;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO order_migrate;

GRANT USAGE ON SCHEMA public TO order_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO order_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO order_app;
