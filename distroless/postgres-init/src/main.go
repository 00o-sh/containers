// postgres-init creates a role and database(s) on a PostgreSQL server,
// driven entirely by INIT_POSTGRES_* environment variables.
//
// It is a shell-free rewrite of apps/postgres-init/entrypoint.sh for the
// distroless image: it execs the same postgres client binaries
// (pg_isready, psql, createuser, createdb) with the same arguments the
// bash script used, so behavior — including INIT_POSTGRES_USER_FLAGS
// passthrough to createuser and /initdb/<dbname>.sql seeding — is
// identical. One deliberate deviation: the bash script had no `set -e`,
// so a failed psql/createuser call was silently ignored; here any
// client-command failure exits non-zero, because an init container that
// half-succeeds silently is worse than one that fails loudly.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// version is stamped by the melange build via -ldflags.
var version = "dev"

func logf(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// run executes a postgres client command with PG* connection env set,
// streaming its output through.
func run(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// query runs a psql command and returns its trimmed stdout (empty when
// the query matched no rows — same contract the bash script relied on).
func query(env []string, command string) (string, error) {
	cmd := exec.Command("psql", "--tuples-only", "--csv", "--command", command)
	cmd.Env = env
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func must(err error, what string) {
	if err != nil {
		logf("%s failed: %v", what, err)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("postgres-init " + version)
		return
	}

	superUser := getenvDefault("INIT_POSTGRES_SUPER_USER", "postgres")
	port := getenvDefault("INIT_POSTGRES_PORT", "5432")
	utf8 := getenvDefault("INIT_POSTGRES_UTF8", "false")

	host := os.Getenv("INIT_POSTGRES_HOST")
	superPass := os.Getenv("INIT_POSTGRES_SUPER_PASS")
	user := os.Getenv("INIT_POSTGRES_USER")
	pass := os.Getenv("INIT_POSTGRES_PASS")
	dbnames := os.Getenv("INIT_POSTGRES_DBNAME")

	if host == "" || superPass == "" || user == "" || pass == "" || dbnames == "" {
		logf("Invalid configuration - missing a required environment variable")
		for _, v := range []struct{ name, val string }{
			{"INIT_POSTGRES_HOST", host},
			{"INIT_POSTGRES_SUPER_PASS", superPass},
			{"INIT_POSTGRES_USER", user},
			{"INIT_POSTGRES_PASS", pass},
			{"INIT_POSTGRES_DBNAME", dbnames},
		} {
			if v.val == "" {
				logf("%s: unset", v.name)
			}
		}
		os.Exit(1)
	}

	env := append(os.Environ(),
		"PGHOST="+host,
		"PGUSER="+superUser,
		"PGPASSWORD="+superPass,
		"PGPORT="+port,
	)

	for run(env, "pg_isready") != nil {
		logf("Waiting for Host '%s' on port '%s' ...", host, port)
		time.Sleep(time.Second)
	}

	userExists, err := query(env, fmt.Sprintf("SELECT 1 FROM pg_roles WHERE rolname = '%s'", user))
	must(err, "role lookup")
	if userExists == "" {
		logf("Create User %s ...", user)
		// INIT_POSTGRES_USER_FLAGS is whitespace-split and passed
		// through verbatim, exactly like the unquoted bash expansion
		// (e.g. "--createdb --createrole").
		args := strings.Fields(os.Getenv("INIT_POSTGRES_USER_FLAGS"))
		args = append(args, user)
		must(run(env, "createuser", args...), "createuser")
	}

	logf("Update password for user %s ...", user)
	must(run(env, "psql", "--command",
		fmt.Sprintf(`alter user "%s" with encrypted password '%s';`, user, pass)), "password update")

	for _, dbname := range strings.Fields(dbnames) {
		dbExists, err := query(env, fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = '%s'", dbname))
		must(err, "database lookup")
		if dbExists == "" {
			if utf8 == "true" {
				logf("Create Database %s with UTF8 encoding ...", dbname)
				must(run(env, "createdb", "--template", "template0", "--encoding", "UTF8",
					"--owner", user, dbname), "createdb")
			} else {
				logf("Create Database %s ...", dbname)
				must(run(env, "createdb", "--owner", user, dbname), "createdb")
			}
			initFile := "/initdb/" + dbname + ".sql"
			if _, err := os.Stat(initFile); err == nil {
				logf("Initialize Database ...")
				must(run(env, "psql", "--dbname", dbname, "--echo-all", "--file", initFile), "database init")
			}
		}
		logf("Update User Privileges on Database ...")
		must(run(env, "psql", "--command",
			fmt.Sprintf(`grant all privileges on database "%s" to "%s";`, dbname, user)), "grant")
	}
}
