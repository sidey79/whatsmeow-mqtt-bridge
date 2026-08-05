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
\if :{?bridge_database}
\else
  \echo 'Missing psql variable bridge_database'
  \quit 2
\endif

SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'bridge_user', :'bridge_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'bridge_user')
\gexec

SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', :'bridge_user', :'bridge_password')
\gexec

SELECT format('CREATE DATABASE %I OWNER %I', :'bridge_database', :'bridge_user')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'bridge_database')
\gexec

SELECT format('ALTER DATABASE %I OWNER TO %I', :'bridge_database', :'bridge_user')
\gexec

SELECT format('GRANT CONNECT, TEMPORARY ON DATABASE %I TO %I', :'bridge_database', :'bridge_user')
\gexec
