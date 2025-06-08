# PostgreSQL Configuration

This directory contains the PostgreSQL configuration for the Yamata no Orochi application.

## Files

- `init.sql` - Template for initial database setup (runs in postgres database context)
- `init-database.sql` - Template for database-specific setup (runs in target database context)
- `init-local-processed.sql` - Processed init.sql for local environment (generated, do not edit)
- `init-beta-processed.sql` - Processed init.sql for beta environment (generated, do not edit)
- `init-database-local-processed.sql` - Processed init-database.sql for local environment (generated, do not edit)
- `init-database-beta-processed.sql` - Processed init-database.sql for beta environment (generated, do not edit)
- `process-init-local.sh` - Script to process both init files for local environment
- `process-init-beta.sh` - Script to process both init files for beta environment
- `postgresql.conf` - PostgreSQL configuration file
- `README.md` - This file

## Two-Step Initialization Process

PostgreSQL initialization is split into two phases to handle the correct execution context:

### Phase 1: Database Creation (`init.sql`)
- Runs in the `postgres` database context
- Creates required extensions (uuid-ossp, pgcrypto, pg_stat_statements)
- Creates the target database if it doesn't exist
- Grants CONNECT permission on the target database
- Sets global PostgreSQL configuration

### Phase 2: Database Setup (`init-database.sql`)
- Runs in the target database context (after it's created)
- Creates audit schema
- Grants schema permissions to the application user
- Sets default privileges for future tables
- Creates utility functions (trigger_set_timestamp)
- Grants execute permissions on functions

## Environment Variable Substitution

Both files contain environment variable placeholders that need to be substituted:

```sql
-- Examples from init files
CREATE DATABASE "${DB_NAME:-yamata_no_orochi}";
GRANT CONNECT ON DATABASE "${DB_NAME:-yamata_no_orochi}" TO "${DB_USER:-yamata_user}";
```

These placeholders are processed by environment-specific scripts:

- **`process-init-local.sh`**: Sources `.env.local` and creates local processed files
- **`process-init-beta.sh`**: Sources `.env.beta` and creates beta processed files

## Usage

### Local Environment
```bash
# Process both init files for local environment
./docker/postgres/process-init-local.sh

# Process with specific environment variables
DB_NAME=my_database DB_USER=my_user ./docker/postgres/process-init-local.sh
```

### Beta Environment
```bash
# Process both init files for beta environment
./docker/postgres/process-init-beta.sh

# Process with specific environment variables
DB_NAME=my_database DB_USER=my_user ./docker/postgres/process-init-beta.sh
```

### Automatic Processing
The deployment scripts automatically run the appropriate processing script:
- **`deploy-local.sh`**: Runs `process-init-local.sh`
- **`deploy-beta.sh`**: Runs `process-init-beta.sh`

## Docker Compose Integration

Each environment mounts both processed files in the correct order:

### Local Environment
```yaml
# docker-compose.local.yml
volumes:
  - ./docker/postgres/init-local-processed.sql:/docker-entrypoint-initdb.d/01-init.sql:ro
  - ./docker/postgres/init-database-local-processed.sql:/docker-entrypoint-initdb.d/02-init-database.sql:ro
```

### Beta Environment
```yaml
# docker-compose.beta.yml
volumes:
  - ./docker/postgres/init-beta-processed.sql:/docker-entrypoint-initdb.d/01-init.sql:ro
  - ./docker/postgres/init-database-beta-processed.sql:/docker-entrypoint-initdb.d/02-init-database.sql:ro
```

**Note**: The numbered prefixes (01-, 02-) ensure proper execution order.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_NAME` | `yamata_no_orochi` | Database name to create |
| `DB_USER` | `yamata_user` | Database user for permissions |

## Notes

- Processed files are generated and should not be edited manually
- Each environment has its own processed files to avoid conflicts
- The two-step process ensures proper execution context for each operation
- Scripts automatically fall back to defaults if no environment file is found
- The original template files are shared between environments
- Environment-specific processing ensures proper variable substitution for each deployment 