#!/usr/bin/env python3
"""Test database connectivity and run basic diagnostics.

Usage:
    python db_connect.py sqlite:///path/to/db.sqlite
    python db_connect.py postgres://user:pass@host:5432/dbname
    python db_connect.py mysql://user:pass@host:3306/dbname
    python db_connect.py --url "$DATABASE_URL"
    python db_connect.py sqlite:///data.db --query "SELECT count(*) FROM users"
    python db_connect.py sqlite:///data.db --tables           # list all tables
    python db_connect.py sqlite:///data.db --schema users     # show table schema

Environment:
    DATABASE_URL – Connection string (alternative to positional arg)

Requirements:
    SQLite: Python stdlib (always available)
    PostgreSQL: psycopg2 (pip install psycopg2-binary)
    MySQL: mysql-connector (pip install mysql-connector-python)
"""

import argparse
import json
import os
import sqlite3
import sys
import time
from urllib.parse import urlparse


def connect_sqlite(path: str):
    """Connect to SQLite database."""
    conn = sqlite3.connect(path)
    conn.row_factory = sqlite3.Row
    return conn


def connect_postgres(url: str):
    """Connect to PostgreSQL database."""
    try:
        import psycopg2
        import psycopg2.extras
    except ImportError:
        raise ImportError("psycopg2 not installed. Run: pip install psycopg2-binary")
    conn = psycopg2.connect(url)
    return conn


def connect_mysql(url: str):
    """Connect to MySQL database."""
    try:
        import mysql.connector
    except ImportError:
        raise ImportError("mysql-connector not installed. Run: pip install mysql-connector-python")
    parsed = urlparse(url)
    conn = mysql.connector.connect(
        host=parsed.hostname,
        port=parsed.port or 3306,
        user=parsed.username,
        password=parsed.password,
        database=parsed.path.lstrip("/"),
    )
    return conn


def get_connection(url: str):
    """Parse URL and return appropriate connection."""
    if url.startswith("sqlite:///"):
        path = url.replace("sqlite:///", "")
        return connect_sqlite(path), "sqlite"
    elif url.startswith("postgres://") or url.startswith("postgresql://"):
        return connect_postgres(url), "postgres"
    elif url.startswith("mysql://"):
        return connect_mysql(url), "mysql"
    elif url.endswith(".db") or url.endswith(".sqlite") or url.endswith(".sqlite3"):
        return connect_sqlite(url), "sqlite"
    else:
        raise ValueError(f"Unsupported database URL scheme: {url}")


def list_tables(conn, db_type: str) -> list[str]:
    """List all tables in the database."""
    cur = conn.cursor()
    if db_type == "sqlite":
        cur.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
    elif db_type == "postgres":
        cur.execute(
            "SELECT table_name FROM information_schema.tables "
            "WHERE table_schema = 'public' ORDER BY table_name"
        )
    elif db_type == "mysql":
        cur.execute("SHOW TABLES")
    rows = cur.fetchall()
    return [row[0] for row in rows]


def table_schema(conn, db_type: str, table_name: str) -> list[dict]:
    """Get column schema for a table."""
    cur = conn.cursor()
    if db_type == "sqlite":
        cur.execute(f"PRAGMA table_info('{table_name}')")
        cols = cur.fetchall()
        return [
            {"name": c[1], "type": c[2], "notnull": bool(c[3]), "default": c[4], "pk": bool(c[5])}
            for c in cols
        ]
    elif db_type == "postgres":
        cur.execute(
            "SELECT column_name, data_type, is_nullable, column_default "
            "FROM information_schema.columns "
            "WHERE table_name = %s ORDER BY ordinal_position",
            (table_name,),
        )
        return [
            {"name": r[0], "type": r[1], "nullable": r[2] == "YES", "default": r[3]}
            for r in cur.fetchall()
        ]
    elif db_type == "mysql":
        cur.execute(f"DESCRIBE `{table_name}`")
        return [
            {"name": r[0], "type": r[1], "nullable": r[2] == "YES", "key": r[3], "default": r[4]}
            for r in cur.fetchall()
        ]
    return []


def run_query(conn, db_type: str, query: str) -> list[dict]:
    """Execute a query and return results as list of dicts."""
    cur = conn.cursor()
    cur.execute(query)
    if cur.description:
        columns = [desc[0] for desc in cur.description]
        return [dict(zip(columns, row)) for row in cur.fetchall()]
    return [{"affected_rows": cur.rowcount}]


def main():
    parser = argparse.ArgumentParser(description="Database connectivity tester")
    parser.add_argument("url", nargs="?", help="Database connection URL")
    parser.add_argument("--url", dest="url_flag", help="Database URL (alternative)")
    parser.add_argument("--tables", action="store_true", help="List all tables")
    parser.add_argument("--schema", metavar="TABLE", help="Show schema for a table")
    parser.add_argument("--query", "-q", help="Execute a SQL query")
    parser.add_argument("--count", metavar="TABLE", help="Count rows in a table")
    args = parser.parse_args()

    url = args.url or args.url_flag or os.environ.get("DATABASE_URL", "")
    if not url:
        parser.error("Database URL required (positional arg, --url, or DATABASE_URL env var)")

    start = time.time()
    try:
        conn, db_type = get_connection(url)
        elapsed = round((time.time() - start) * 1000, 1)
    except Exception as e:
        print(json.dumps({
            "status": "error",
            "error": str(e),
            "url": url[:url.find("@")] + "@***" if "@" in url else url,
        }, indent=2))
        sys.exit(1)

    # Mask credentials in output
    safe_url = url[:url.find("@")] + "@***" if "@" in url else url

    if args.tables:
        tables = list_tables(conn, db_type)
        print(json.dumps({"status": "ok", "db_type": db_type, "tables": tables, "count": len(tables)}, indent=2))
    elif args.schema:
        schema = table_schema(conn, db_type, args.schema)
        print(json.dumps({"status": "ok", "table": args.schema, "columns": schema}, indent=2))
    elif args.query:
        try:
            results = run_query(conn, db_type, args.query)
            print(json.dumps({"status": "ok", "rows": len(results), "data": results}, indent=2))
        except Exception as e:
            print(json.dumps({"status": "error", "error": str(e)}, indent=2))
            sys.exit(1)
    elif args.count:
        results = run_query(conn, db_type, f"SELECT count(*) as row_count FROM {args.count}")
        print(json.dumps({"status": "ok", "table": args.count, "row_count": results[0]["row_count"]}, indent=2))
    else:
        # Connection test with basic diagnostics
        tables = list_tables(conn, db_type)
        result = {
            "status": "ok",
            "db_type": db_type,
            "url": safe_url,
            "connect_ms": elapsed,
            "table_count": len(tables),
            "tables": tables[:20],  # cap at 20 for display
        }
        if len(tables) > 20:
            result["tables_truncated"] = True
        print(json.dumps(result, indent=2))

    conn.close()


if __name__ == "__main__":
    main()
