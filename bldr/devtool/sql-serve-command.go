//go:build !js

package devtool

import (
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"

	"github.com/aperturerobotics/cli"
	mysql_devserver "github.com/s4wave/spacewave/db/sql/mysql/devserver"
)

// sqlServeArgs are the arguments of the sql-serve command.
type sqlServeArgs struct {
	// listen is the TCP address the endpoint listens on.
	listen string
	// database selects the database the schema builds.
	database string
	// schema lists the .sql files and directories applied in order.
	schema cli.StringSlice
}

// BuildSqlServeCommand builds the sql-serve command.
func BuildSqlServeCommand() *cli.Command {
	args := &sqlServeArgs{}
	return &cli.Command{
		Name:      "sql-serve",
		Usage:     "serve an in-memory Spacewave SQL database built from schema files",
		ArgsUsage: "[-- command args...]",
		Description: "Builds a Spacewave SQL database from the --schema files in memory and serves\n" +
			"it over the MySQL protocol, for tools that type queries by preparing them,\n" +
			"such as Reylo. A directory contributes its .sql files in name order. With a\n" +
			"command, runs it with DATABASE_URL set to the endpoint and exits with it;\n" +
			"without one, prints DATABASE_URL and serves until interrupted.\n\n" +
			"Example, a type check:\n" +
			"  bldr sql-serve --schema migrations -- tsrs --noEmit --runExternalCode",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// BuildFlags returns the sql-serve flags.
func (a *sqlServeArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			Name:        "schema",
			Usage:       ".sql file or directory of migrations to apply, in order",
			Required:    true,
			Destination: &a.schema,
		},
		&cli.StringFlag{
			Name:        "database",
			Usage:       "name of the database the schema builds",
			Value:       "app",
			Destination: &a.database,
		},
		&cli.StringFlag{
			Name:        "listen",
			Usage:       "TCP address to listen on",
			Value:       "127.0.0.1:0",
			Destination: &a.listen,
		},
	}
}

// Run builds and serves the database, then runs the command or waits.
func (a *sqlServeArgs) Run(c *cli.Context) error {
	// Read the schema before opening the listener.
	_, schema, err := mysql_devserver.ReadSchemaFiles(a.schema.Value())
	if err != nil {
		return err
	}

	// Start the endpoint on the listener.
	lis, err := net.Listen("tcp", a.listen)
	if err != nil {
		return err
	}
	srv, err := mysql_devserver.Start(c.Context, lis, a.database, schema)
	if err != nil {
		return err
	}
	defer srv.Close()

	// Run the command against the endpoint when one is given.
	if c.NArg() != 0 {
		cmd := exec.CommandContext(c.Context, c.Args().First(), c.Args().Tail()...) //nolint:gosec // running the caller's command is the point of sql-serve.
		cmd.Env = append(os.Environ(), "DATABASE_URL="+srv.URL())
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return cli.Exit("", exitErr.ExitCode())
			}
			return err
		}
		return nil
	}

	// Otherwise print the endpoint and serve until interrupted.
	_, _ = io.WriteString(os.Stdout, "DATABASE_URL="+srv.URL()+"\n")
	ctx, stop := signal.NotifyContext(c.Context, os.Interrupt)
	defer stop()
	<-ctx.Done()
	return nil
}
