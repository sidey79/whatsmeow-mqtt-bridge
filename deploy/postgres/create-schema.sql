\set ON_ERROR_STOP on

\if :{?bridge_user}
\else
  \echo 'Missing psql variable bridge_user'
  \quit 2
\endif
\if :{?bridge_password}
\else
  \echo 'Missing psql variable bridge_password'
  \quit 2
\endif
\if :{?bridge_schema}
\else
  \echo 'Missing psql variable bridge_schema'
  \quit 2
\endif

SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'bridge_user', :'bridge_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'bridge_user')
\gexec

SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', :'bridge_user', :'bridge_password')
\gexec

SELECT format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), :'bridge_user')
\gexec

SELECT format('CREATE SCHEMA IF NOT EXISTS %I AUTHORIZATION %I', :'bridge_schema', :'bridge_user')
\gexec

SELECT format('ALTER SCHEMA %I OWNER TO %I', :'bridge_schema', :'bridge_user')
\gexec

SELECT format('GRANT USAGE, CREATE ON SCHEMA %I TO %I', :'bridge_schema', :'bridge_user')
\gexec
